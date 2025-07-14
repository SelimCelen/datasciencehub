package app

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/yaml.v3"
)

func (app *AppContext) uploadData(c *gin.Context) {
	var inputData interface{}
	if err := c.ShouldBindJSON(&inputData); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	job := DataJob{
		Name:        fmt.Sprintf("Job-%d", time.Now().Unix()),
		Description: "Uploaded data job",
		InputData:   inputData,
		Status:      "uploaded",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := collection.InsertOne(ctx, job)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, gin.H{"id": result.InsertedID, "message": "Data uploaded successfully"})
}

func (app *AppContext) processData(c *gin.Context) {
	var request struct {
		JobID   string `json:"job_id"`
		Plugins []struct {
			Name   string                 `json:"name"`
			Params map[string]interface{} `json:"params"`
		} `json:"plugins"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	objID, err := primitive.ObjectIDFromHex(request.JobID)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid job ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	var job DataJob
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&job)
	if err != nil {
		c.JSON(404, gin.H{"error": "job not found"})
		return
	}

	results := make(map[string]interface{})
	data := job.InputData

	for _, plugin := range request.Plugins {
		app.PluginsMux.RLock()
		script, exists := app.Plugins[plugin.Name]
		app.PluginsMux.RUnlock()

		if !exists {
			results[plugin.Name] = gin.H{"error": "plugin not found"}
			continue
		}

		output, err := app.runScript(script, data, plugin.Params)
		if err != nil {
			results[plugin.Name] = gin.H{"error": err.Error()}
			continue
		}

		results[plugin.Name] = output
		data = output
	}

	update := bson.M{
		"$set": bson.M{
			"status":     "processed",
			"results":    results,
			"updated_at": time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Data processed successfully", "results": results})
}

func (app *AppContext) processYamlTask(c *gin.Context) {
	file, err := c.FormFile("yaml_file")
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	yamlFile, err := file.Open()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer yamlFile.Close()

	yamlData, err := io.ReadAll(yamlFile)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	var task TaskDefinition
	if err := yaml.Unmarshal(yamlData, &task); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	taskCollection := app.MongoClient.Database(app.Config.DatabaseName).Collection("tasks")
	_, err = taskCollection.InsertOne(ctx, task)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	results := make(map[string]interface{})
	var wg sync.WaitGroup
	var mutex sync.Mutex

	sem := make(chan struct{}, app.Config.MaxParallel)

	processStep := func(_ int, step map[string]interface{}, data interface{}) (interface{}, error) {
		pluginName, ok := step["plugin"].(string)
		if !ok {
			return nil, fmt.Errorf("plugin name not specified in step")
		}

		params, ok := step["params"].(map[string]interface{})
		if !ok {
			params = make(map[string]interface{})
		}

		app.PluginsMux.RLock()
		script, exists := app.Plugins[pluginName]
		app.PluginsMux.RUnlock()

		if !exists {
			return nil, fmt.Errorf("plugin %s not found", pluginName)
		}

		return app.runScript(script, data, params)
	}

	// Get inputData from first step if exists and references job_id
	var inputData interface{}
	if len(task.Steps) > 0 {
		if inputRef, ok := task.Steps[0]["input"].(map[string]interface{}); ok {
			if jobID, ok := inputRef["job_id"].(string); ok {
				objID, err := primitive.ObjectIDFromHex(jobID)
				if err != nil {
					c.JSON(400, gin.H{"error": "invalid job ID in input reference"})
					return
				}

				ctxJob, cancelJob := context.WithTimeout(c.Request.Context(), 10*time.Second)
				defer cancelJob()

				jobCollection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
				var job DataJob
				err = jobCollection.FindOne(ctxJob, bson.M{"_id": objID}).Decode(&job)
				if err != nil {
					c.JSON(404, gin.H{"error": "referenced job not found"})
					return
				}

				inputData = job.InputData
			}
		}
	}

	currentData := inputData
	if task.Parallel {
		for i, step := range task.Steps {
			wg.Add(1)
			go func(stepNum int, step map[string]interface{}) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				stepName := fmt.Sprintf("step_%d", stepNum)
				if name, ok := step["name"].(string); ok {
					stepName = name
				}

				result, err := processStep(stepNum, step, inputData)
				if err != nil {
					mutex.Lock()
					results[stepName] = gin.H{"error": err.Error()}
					mutex.Unlock()
					return
				}

				mutex.Lock()
				results[stepName] = result
				mutex.Unlock()
			}(i, step)
		}
		wg.Wait()
	} else {
		for i, step := range task.Steps {
			stepName := fmt.Sprintf("step_%d", i)
			if name, ok := step["name"].(string); ok {
				stepName = name
			}

			result, err := processStep(i, step, currentData)
			if err != nil {
				results[stepName] = gin.H{"error": err.Error()}
				break
			}

			results[stepName] = result
			currentData = result
		}
	}

	jobCtx, cancelJob := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancelJob()

	jobCollection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	job := DataJob{
		Name:        task.Name,
		Description: task.Description,
		InputData:   inputData,
		Status:      "processed",
		Results:     results,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := jobCollection.InsertOne(jobCtx, job)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"message": "YAML task processed successfully",
		"job_id":  result.InsertedID,
		"results": results,
	})
}

func (app *AppContext) listJobs(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	var jobs []DataJob
	if err = cursor.All(ctx, &jobs); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, jobs)
}

func (app *AppContext) getJob(c *gin.Context) {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid job ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	var job DataJob
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&job)
	if err != nil {
		c.JSON(404, gin.H{"error": "job not found"})
		return
	}

	c.JSON(200, job)
}

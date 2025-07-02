package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robertkrimen/otto"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/yaml.v3"
)

// Configuration structures
type ServerConfig struct {
	Port         string        `yaml:"port" bson:"port"`
	MongoURI     string        `yaml:"mongo_uri" bson:"mongo_uri"`
	DatabaseName string        `yaml:"database_name" bson:"database_name"`
	JSTimeout    time.Duration `yaml:"js_timeout" bson:"js_timeout"`
	MaxParallel  int           `yaml:"max_parallel" bson:"max_parallel"`
}

type Plugin struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Name        string             `bson:"name"`
	Description string             `bson:"description"`
	JavaScript  string             `bson:"javascript"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}

type DataJob struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Name        string             `bson:"name"`
	Description string             `bson:"description"`
	InputData   interface{}        `bson:"input_data"`
	Status      string             `bson:"status"`
	Results     interface{}        `bson:"results"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}

type TaskDefinition struct {
	Name        string                   `yaml:"name" bson:"name"`
	Description string                   `yaml:"description" bson:"description"`
	Steps       []map[string]interface{} `yaml:"steps" bson:"steps"`
	Parallel    bool                     `yaml:"parallel" bson:"parallel"`
}

// AppContext holds the application context
type AppContext struct {
	Config      ServerConfig
	MongoClient *mongo.Client
	Router      *gin.Engine
	Plugins     map[string]*otto.Script
	VMFactory   func() *otto.Otto
	PluginsMux  sync.RWMutex
}

// Initialize the application
func (app *AppContext) Initialize() {
	// Load configuration
	app.loadConfig()

	// Initialize MongoDB
	app.initMongoDB()
	app.createIndexes()

	// Initialize JavaScript VM
	app.VMFactory = func() *otto.Otto {
		vm := otto.New()
		// Disable dangerous functions
		vm.Set("import", nil)
		vm.Set("load", nil)
		vm.Set("require", nil)
		return vm
	}
	app.Plugins = make(map[string]*otto.Script)

	// Load plugins from DB
	app.loadPlugins()

	// Initialize router
	app.initRouter()
}

func (app *AppContext) loadConfig() {
	// Default configuration
	app.Config = ServerConfig{
		Port:         "8080",
		MongoURI:     "mongodb://localhost:27017",
		DatabaseName: "scientific_data_processing",
		JSTimeout:    5 * time.Second,
		MaxParallel:  10,
	}

	// Try to load from YAML file
	if _, err := os.Stat("config.yaml"); err == nil {
		data, err := os.ReadFile("config.yaml")
		if err != nil {
			log.Fatalf("Error reading config file: %v", err)
		}

		err = yaml.Unmarshal(data, &app.Config)
		if err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	}

	// Try to load from environment variables
	if port := os.Getenv("SERVER_PORT"); port != "" {
		app.Config.Port = port
	}
	if mongoURI := os.Getenv("MONGO_URI"); mongoURI != "" {
		app.Config.MongoURI = mongoURI
	}
	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		app.Config.DatabaseName = dbName
	}
	if timeout := os.Getenv("JS_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			app.Config.JSTimeout = d
		}
	}
	if maxParallel := os.Getenv("MAX_PARALLEL"); maxParallel != "" {
		if n, err := fmt.Sscanf(maxParallel, "%d", &app.Config.MaxParallel); n == 1 && err == nil {
			if app.Config.MaxParallel < 1 {
				app.Config.MaxParallel = 10
			}
		}
	}
}

func (app *AppContext) initMongoDB() {
	clientOptions := options.Client().ApplyURI(app.Config.MongoURI)
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	app.MongoClient = client
}

func (app *AppContext) createIndexes() {
	db := app.MongoClient.Database(app.Config.DatabaseName)
	
	// Index for plugins collection
	_, err := db.Collection("plugins").Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys:    bson.M{"name": 1},
			Options: options.Index().SetUnique(true),
		},
	)
	if err != nil {
		log.Printf("Error creating plugin index: %v", err)
	}

	// Index for data_jobs collection
	_, err = db.Collection("data_jobs").Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys: bson.M{"name": 1},
		},
	)
	if err != nil {
		log.Printf("Error creating job index: %v", err)
	}
}

func (app *AppContext) loadPlugins() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Error loading plugins: %v", err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var plugin Plugin
		if err := cursor.Decode(&plugin); err != nil {
			log.Printf("Error decoding plugin: %v", err)
			continue
		}

		vm := app.VMFactory()
		script, err := vm.Compile("", plugin.JavaScript)
		if err != nil {
			log.Printf("Error compiling plugin %s: %v", plugin.Name, err)
			continue
		}

		app.PluginsMux.Lock()
		app.Plugins[plugin.Name] = script
		app.PluginsMux.Unlock()
	}
}

func (app *AppContext) initRouter() {
	app.Router = gin.Default()
	app.Router.Use(func(c *gin.Context) {
		c.Set("start", time.Now())
		c.Next()
	})

	// API routes
	api := app.Router.Group("/api/v1")
	{
		api.POST("/data/upload", app.uploadData)
		api.POST("/data/process", app.processData)
		api.GET("/data/jobs", app.listJobs)
		api.GET("/data/jobs/:id", app.getJob)
		api.POST("/data/process/yaml", app.processYamlTask)

		// Plugin management endpoints
		api.POST("/plugins", app.uploadPlugin)
		api.GET("/plugins", app.listPlugins)
		api.GET("/plugins/:name", app.getPlugin)
		api.DELETE("/plugins/:name", app.deletePlugin)
		api.POST("/plugins/:name/execute", app.executePlugin)
	}
}

// Helper function to run JavaScript with timeout
func (app *AppContext) runScript(script *otto.Script, input interface{}, params map[string]interface{}) (interface{}, error) {
	vm := app.VMFactory()
	vm.Set("input", input)
	vm.Set("params", params)

	done := make(chan struct{})
	var value otto.Value
	var err error

	go func() {
		defer close(done)
		value, err = vm.Run(script)
	}()

	select {
	case <-done:
		if err != nil {
			return nil, err
		}
		return value.Export()
	case <-time.After(app.Config.JSTimeout):
		return nil, fmt.Errorf("execution timed out after %v", app.Config.JSTimeout)
	}
}

// API Handlers
func (app *AppContext) uploadData(c *gin.Context) {
	var inputData interface{}
	if err := c.ShouldBindJSON(&inputData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      result.InsertedID,
		"message": "Data uploaded successfully",
	})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert string ID to ObjectID
	objID, err := primitive.ObjectIDFromHex(request.JobID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get the job from database
	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	var job DataJob
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&job)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// Process data with plugins
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
		data = output // Chain the output to next plugin
	}

	// Update job with results
	update := bson.M{
		"$set": bson.M{
			"status":     "processed",
			"results":    results,
			"updated_at": time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Data processed successfully",
		"results": results,
	})
}

func (app *AppContext) processYamlTask(c *gin.Context) {
	file, err := c.FormFile("yaml_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Open the uploaded file
	yamlFile, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer yamlFile.Close()

	// Read YAML content
	yamlData, err := io.ReadAll(yamlFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse YAML
	var task TaskDefinition
	err = yaml.Unmarshal(yamlData, &task)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Store the task definition in MongoDB
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	taskCollection := app.MongoClient.Database(app.Config.DatabaseName).Collection("tasks")
	_, err = taskCollection.InsertOne(ctx, task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Process the task
	results := make(map[string]interface{})
	var wg sync.WaitGroup
	var mutex sync.Mutex

	// Worker pool for parallel execution
	sem := make(chan struct{}, app.Config.MaxParallel)

	processStep := func(stepNum int, step map[string]interface{}, data interface{}) (interface{}, error) {
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

	// Get input data if specified
	var inputData interface{}
	if len(task.Steps) > 0 {
		if inputRef, ok := task.Steps[0]["input"].(map[string]interface{}); ok {
			if jobID, ok := inputRef["job_id"].(string); ok {
				objID, err := primitive.ObjectIDFromHex(jobID)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID in input reference"})
					return
				}

				ctxJob, cancelJob := context.WithTimeout(c.Request.Context(), 10*time.Second)
				defer cancelJob()

				jobCollection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
				var job DataJob
				err = jobCollection.FindOne(ctxJob, bson.M{"_id": objID}).Decode(&job)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "referenced job not found"})
					return
				}

				inputData = job.InputData
			}
		}
	}

	// Process steps
	currentData := inputData
	if task.Parallel {
		// Parallel execution with worker pool
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
		// Sequential execution
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

	// Create a job record with the results
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	var jobs []DataJob
	if err := cursor.All(ctx, &jobs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

func (app *AppContext) getJob(c *gin.Context) {
	objID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("data_jobs")
	var job DataJob
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&job)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

func (app *AppContext) uploadPlugin(c *gin.Context) {
	var plugin Plugin
	if err := c.ShouldBindJSON(&plugin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate JavaScript
	vm := app.VMFactory()
	_, err := vm.Compile("", plugin.JavaScript)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("JavaScript compilation error: %v", err)})
		return
	}

	// Set timestamps
	plugin.CreatedAt = time.Now()
	plugin.UpdatedAt = time.Now()

	// Store in MongoDB
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	result, err := collection.InsertOne(ctx, plugin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Cache the compiled script
	script, _ := vm.Compile("", plugin.JavaScript)
	app.PluginsMux.Lock()
	app.Plugins[plugin.Name] = script
	app.PluginsMux.Unlock()

	c.JSON(http.StatusCreated, gin.H{
		"id":      result.InsertedID,
		"message": "Plugin uploaded successfully",
	})
}

func (app *AppContext) listPlugins(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	var plugins []Plugin
	if err := cursor.All(ctx, &plugins); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plugins)
}

func (app *AppContext) getPlugin(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	var plugin Plugin
	err := collection.FindOne(ctx, bson.M{"name": c.Param("name")}).Decode(&plugin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}

	c.JSON(http.StatusOK, plugin)
}

func (app *AppContext) deletePlugin(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	result, err := collection.DeleteOne(ctx, bson.M{"name": c.Param("name")})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}

	// Remove from cache
	app.PluginsMux.Lock()
	delete(app.Plugins, c.Param("name"))
	app.PluginsMux.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Plugin deleted successfully"})
}

func (app *AppContext) executePlugin(c *gin.Context) {
	var request struct {
		Input  interface{}            `json:"input"`
		Params map[string]interface{} `json:"params"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pluginName := c.Param("name")

	app.PluginsMux.RLock()
	script, exists := app.Plugins[pluginName]
	app.PluginsMux.RUnlock()
	
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}

	output, err := app.runScript(script, request.Input, request.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"output": output,
	})
}

func main() {
	app := AppContext{}
	app.Initialize()

	server := &http.Server{
		Addr:    ":" + app.Config.Port,
		Handler: app.Router,
	}

	// Graceful shutdown setup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s", app.Config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	log.Println("Shutting down server...")

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Disconnect MongoDB
	if err := app.MongoClient.Disconnect(ctx); err != nil {
		log.Fatalf("MongoDB disconnect error: %v", err)
	}

	log.Println("Server exited properly")
}
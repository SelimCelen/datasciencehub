package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (app *AppContext) uploadPlugin(c *gin.Context) {
	var input struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		JavaScript  string `json:"javascript" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate JavaScript before storing
	if _, err := goja.Compile("", input.JavaScript, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid JavaScript: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// Create GridFS bucket
	bucket, err := gridfs.NewBucket(app.MongoClient.Database(app.Config.DatabaseName))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create GridFS bucket"})
		return
	}

	// Delete existing file if it exists (GridFS allows multiple files with same name)
	if err := bucket.Delete(strings.TrimSpace(input.Name)); err != nil && !errors.Is(err, gridfs.ErrFileNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clean existing plugin"})
		return
	}

	// Upload to GridFS
	uploadStream, err := bucket.OpenUploadStream(strings.TrimSpace(input.Name))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload stream"})
		return
	}
	defer uploadStream.Close()

	if _, err := uploadStream.Write([]byte(input.JavaScript)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write plugin content"})
		return
	}

	// Store metadata in plugins collection
	plugin := Plugin{
		Name:        strings.TrimSpace(input.Name),
		Description: input.Description,
	}

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	filter := bson.M{"name": plugin.Name}
	update := bson.M{"$set": plugin}
	opts := options.Update().SetUpsert(true)

	if _, err := collection.UpdateOne(ctx, filter, update, opts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update plugin metadata"})
		return
	}

	// Cache the compiled script
	program, err := goja.Compile("", input.JavaScript, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JavaScript after upload: " + err.Error()})
		return
	}

	app.PluginsMux.Lock()
	app.Plugins[plugin.Name] = program
	app.PluginsMux.Unlock()

	c.JSON(http.StatusCreated, gin.H{"message": "plugin uploaded/updated successfully"})
}
func (app *AppContext) listPlugins(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	var plugins []Plugin
	if err = cursor.All(ctx, &plugins); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, plugins)
}

func (app *AppContext) getPlugin(c *gin.Context) {
	name := c.Param("name")

	bucket, err := gridfs.NewBucket(app.MongoClient.Database(app.Config.DatabaseName))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create GridFS bucket"})
		return
	}

	downloadStream, err := bucket.OpenDownloadStreamByName(strings.TrimSpace(name))
	if err != nil {
		if err == gridfs.ErrFileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open download stream"})
		return
	}
	defer downloadStream.Close()

	fileBuffer := bytes.NewBuffer(nil)
	if _, err := io.Copy(fileBuffer, downloadStream); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read plugin content"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": fileBuffer.String()})
}
func (app *AppContext) deletePlugin(c *gin.Context) {
	name := c.Param("name")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")

	_, err := collection.DeleteOne(ctx, bson.M{"name": name})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	app.PluginsMux.Lock()
	delete(app.Plugins, name)
	app.PluginsMux.Unlock()

	c.JSON(200, gin.H{"message": "plugin deleted"})
}

func (app *AppContext) executePlugin(c *gin.Context) {
	name := c.Param("name")

	var input struct {
		Data   interface{}            `json:"data"`
		Params map[string]interface{} `json:"params"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	app.PluginsMux.RLock()
	script, exists := app.Plugins[name]
	app.PluginsMux.RUnlock()

	if !exists {
		c.JSON(404, gin.H{"error": "plugin not found"})
		return
	}

	output, err := app.runScript(script, input.Data, input.Params)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"result": output})
}

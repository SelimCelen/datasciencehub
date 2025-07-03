package app

import (
	"context"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (app *AppContext) uploadPlugin(c *gin.Context) {
	var input struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		JavaScript  string `json:"javascript" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	plugin := Plugin{
		Name:        strings.TrimSpace(input.Name),
		Description: input.Description,
		JavaScript:  input.JavaScript,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")

	// Compile script to check validity
	_, err := goja.Compile("", plugin.JavaScript, false)
	if err != nil {
		// handle error
		c.JSON(400, gin.H{"error": "invalid JavaScript: " + err.Error() + plugin.JavaScript})
		return
	}

	if err != nil {

	}

	// Upsert plugin
	filter := bson.M{"name": plugin.Name}
	update := bson.M{
		"$set": plugin,
	}

	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	app.PluginsMux.Lock()
	defer app.PluginsMux.Unlock()
	program, err := goja.Compile("", plugin.JavaScript, false)
	if err == nil {
		// handle error
		app.Plugins[plugin.Name] = program
	}
	// Compile and cache plugin

	c.JSON(201, gin.H{"message": "plugin uploaded/updated successfully"})
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	collection := app.MongoClient.Database(app.Config.DatabaseName).Collection("plugins")

	var plugin Plugin
	err := collection.FindOne(ctx, bson.M{"name": name}).Decode(&plugin)
	if err != nil {
		c.JSON(404, gin.H{"error": "plugin not found"})
		return
	}

	c.JSON(200, plugin)
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

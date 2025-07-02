package app

import (
	"context"
	"log"
	"time"

	"github.com/dop251/goja"
	"go.mongodb.org/mongo-driver/bson"
)

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

		script, err := goja.Compile("", plugin.JavaScript, false)
		if err != nil {
			log.Printf("Error compiling plugin %s: %v", plugin.Name, err)
			continue
		}

		app.PluginsMux.Lock()
		app.Plugins[plugin.Name] = script
		app.PluginsMux.Unlock()
	}
}

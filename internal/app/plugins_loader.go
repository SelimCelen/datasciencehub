package app

import (
	"bytes"
	"context"
	"io"
	"log"
	"strings"
	"time"

	"github.com/dop251/goja"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	bucket, _ := gridfs.NewBucket(app.MongoClient.Database(app.Config.DatabaseName))
	for cursor.Next(ctx) {
		var plugin Plugin
		if err := cursor.Decode(&plugin); err != nil {
			log.Printf("Error decoding plugin: %v", err)
			continue
		}
		filter := bson.M{"filename": strings.TrimSpace(plugin.Name)}
		_, err := bucket.Find(filter)

		if err != nil {
			log.Printf("Error loading plugin script from Gridfs: %v", err)
			continue
			// handle error
		}
		downloadStream, err := bucket.OpenDownloadStreamByName(strings.TrimSpace(plugin.Name), &options.NameOptions{})
		if err != nil {
			log.Printf("Error loading plugin script from Gridfs: %v", err)
			continue
		}
		fileBuffer := bytes.NewBuffer(nil)
		if _, err := io.Copy(fileBuffer, downloadStream); err != nil {
			// handle error
		}
		io.Copy(fileBuffer, downloadStream)

		pluginAsScript := fileBuffer.String()

		script, err := goja.Compile("", pluginAsScript, false)
		if err != nil {
			log.Printf("Error compiling plugin %s: %v", pluginAsScript, err)
			continue
		}

		app.PluginsMux.Lock()
		app.Plugins[plugin.Name] = script
		app.PluginsMux.Unlock()
	}
}

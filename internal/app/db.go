package app

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

	// Plugins index
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

	// Data jobs index
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

package app

import (
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port         string        `yaml:"port" bson:"port"`
	MongoURI     string        `yaml:"mongo_uri" bson:"mongo_uri"`
	DatabaseName string        `yaml:"database_name" bson:"database_name"`
	JSTimeout    time.Duration `yaml:"js_timeout" bson:"js_timeout"`
	MaxParallel  int           `yaml:"max_parallel" bson:"max_parallel"`
}

func (app *AppContext) loadConfig() {
	app.Config = ServerConfig{
		Port:         "8080",
		MongoURI:     "mongodb://localhost:27017",
		DatabaseName: "scientific_data_processing",
		JSTimeout:    5 * time.Second,
		MaxParallel:  10,
	}

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
		var val int
		n, err := fmt.Sscanf(maxParallel, "%d", &val)
		if n == 1 && err == nil {
			if val >= 1 {
				app.Config.MaxParallel = val
			}
		}
	}
}

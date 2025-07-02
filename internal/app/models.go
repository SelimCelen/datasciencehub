package app

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

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

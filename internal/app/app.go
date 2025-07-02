package app

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/robertkrimen/otto"
	"go.mongodb.org/mongo-driver/mongo"
)

type AppContext struct {
	Config      ServerConfig
	MongoClient *mongo.Client
	Router      *gin.Engine
	Plugins     map[string]*otto.Script
	VMFactory   func() *otto.Otto
	PluginsMux  sync.RWMutex
}

func NewAppContext() *AppContext {
	return &AppContext{
		Plugins: make(map[string]*otto.Script),
	}
}

func (app *AppContext) Initialize() {
	app.loadConfig()
	app.initMongoDB()
	app.createIndexes()
	app.initVMFactory()
	app.loadPlugins()
	app.initRouter()
}

func (app *AppContext) initVMFactory() {
	app.VMFactory = func() *otto.Otto {
		vm := otto.New()
		vm.Set("import", nil)
		vm.Set("load", nil)
		vm.Set("require", nil)
		return vm
	}
}

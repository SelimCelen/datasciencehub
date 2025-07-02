package app

import (
	"sync"

	"github.com/dop251/goja"

	"github.com/gin-gonic/gin"

	"go.mongodb.org/mongo-driver/mongo"
)

type AppContext struct {
	Config      ServerConfig
	MongoClient *mongo.Client
	Router      *gin.Engine
	Plugins     map[string]*goja.Program
	VMFactory   func() *goja.Runtime
	PluginsMux  sync.RWMutex
}

func NewAppContext() *AppContext {
	return &AppContext{
		Plugins: make(map[string]*goja.Program),
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
	app.VMFactory = func() *goja.Runtime {
		vm := goja.New()
		vm.Set("import", nil)
		vm.Set("load", nil)
		vm.Set("require", nil)
		return vm
	}
}

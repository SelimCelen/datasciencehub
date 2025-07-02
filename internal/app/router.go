package app

import (
	"time"

	"github.com/gin-gonic/gin"
)

func (app *AppContext) initRouter() {
	app.Router = gin.Default()
	app.Router.Use(func(c *gin.Context) {
		c.Set("start", time.Now())
		c.Next()
	})

	api := app.Router.Group("/api/v1")
	{
		// Data Jobs
		api.POST("/data/upload", app.uploadData)
		api.POST("/data/process", app.processData)
		api.GET("/data/jobs", app.listJobs)
		api.GET("/data/jobs/:id", app.getJob)
		api.POST("/data/process/yaml", app.processYamlTask)

		// Plugins
		api.POST("/plugins", app.uploadPlugin)
		api.GET("/plugins", app.listPlugins)
		api.GET("/plugins/:name", app.getPlugin)
		api.DELETE("/plugins/:name", app.deletePlugin)
		api.POST("/plugins/:name/execute", app.executePlugin)
	}
}

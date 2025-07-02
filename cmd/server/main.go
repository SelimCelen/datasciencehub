package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SelimCelen/datasciencehub/internal/app"
)

func main() {
	appCtx := app.NewAppContext()
	appCtx.Initialize()

	server := &http.Server{
		Addr:    ":" + appCtx.Config.Port,
		Handler: appCtx.Router,
	}

	// Graceful shutdown setup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s", appCtx.Config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	if err := appCtx.MongoClient.Disconnect(ctx); err != nil {
		log.Fatalf("MongoDB disconnect error: %v", err)
	}

	log.Println("Server exited properly")
}

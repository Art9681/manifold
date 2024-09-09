// main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/jaegertracing"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Define the verbose logging flag
	var verbose bool

	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.Parse()

	// Get the host information
	host := NewHostInfoProvider()
	PrintHostInfo(host)

	// Load the configuration
	config, err := LoadConfig("config.yml")
	if err != nil {
		log.Fatal(err)
	}

	// Get the home folder path
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	// Set the data path
	config.DataPath = home + "/.manifold"

	// Initialize the application
	db, _, err := initializeApplication(config)
	if err != nil {
		log.Fatal(err)
	}

	err = db.AutoMigrate(&ToolMetadata{}, &ToolParam{}, &CompletionsRole{}, &ModelParams{}, &URLTracking{})
	if err != nil {
		log.Fatal(err)
	}

	// Load tools data into the database
	if err := loadToolsToDB(db, config.Tools); err != nil {
		log.Fatal(err)
	}

	// Load completions roles into the database
	if err := loadCompletionsRolesToDB(db, config.Roles); err != nil {
		log.Fatal(err)
	}

	// Load model parameters into the database
	if err := loadModelDataToDB(db, config.LanguageModels); err != nil {
		log.Fatal(err)
	}

	config, err = checkDownloadedModels(db, config)
	if err != nil {
		log.Fatal(err)
	}

	// Create a new Echo instance
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// CORS default - For dev only
	// Allows requests from any origin wth GET, HEAD, PUT, POST or DELETE method.
	e.Use(middleware.CORS())

	// CORS restricted - Production Settings
	// Allows requests from any `https://labstack.com` or `https://labstack.net` origin
	// wth GET, PUT, POST or DELETE method.
	// e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
	// 	AllowOrigins: []string{"https://labstack.com", "https://labstack.net"},
	// 	AllowMethods: []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete},
	// }))

	// Enable tracing middleware
	// https://echo.labstack.com/docs/middleware/jaeger
	c := jaegertracing.New(e, nil)
	defer c.Close()

	// Set up routes
	setupRoutes(e, config)

	// Create a completions service context
	var completionsService *ExternalService
	completionsCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var llmService ServiceConfig
	if config.LLMBackend == "gguf" {
		llmService = config.Services[1]
	} else if config.LLMBackend == "mlx" {
		llmService = config.Services[2]
	}

	completionsService = NewExternalService(llmService, true)

	// Start the completions service
	if err := completionsService.Start(completionsCtx); err != nil {
		e.Logger.Fatal(err)
	}

	// Set up graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		// Cancel the context to signal all operations to stop
		cancel()

		// Stop the MLX service first
		if completionsService != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := completionsService.Stop(ctx); err != nil {
				e.Logger.Info(err)
			}
		}

		// Then shut down the Echo server
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			e.Logger.Error(err)
		}
	}()

	e.Logger.Info(e.Start(fmt.Sprintf(":%d", config.Services[0].Port)))
}

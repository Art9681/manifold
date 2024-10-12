// manifold/main.go

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

	"github.com/blevesearch/bleve/v2"
	"github.com/labstack/echo-contrib/jaegertracing"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var (
	llmClient   LLMClient
	searchIndex bleve.Index
	db          *SQLiteDB
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

	// Initialize the application
	db, err = initializeApplication(config)
	if err != nil {
		log.Fatal(err)
	}

	// Get the list of models from the database
	models, err := db.GetModels()
	if err != nil {
		log.Fatal("Failed to load models:", err)
	}

	config.LanguageModels = models

	// Load the selected model from the database
	selectedModels, err := GetSelectedModels(db.db)
	if err != nil {
		log.Fatal("Failed to load selected model:", err)
	}

	config.SelectedModels = selectedModels

	// Initialize the rest of your application (models, services, etc.)
	// mm := NewModelManager(config.DataPath)
	// modelPaths, err := mm.ScanModels()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// log.Println("Found models:")
	// for _, modelPath := range modelPaths {
	// 	log.Println(modelPath)
	// }

	// Initialize Echo instance
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// CORS default - For dev only
	e.Use(middleware.CORS())

	// Enable tracing middleware
	c := jaegertracing.New(e, nil)
	defer c.Close()

	// Set up routes
	setupRoutes(e, config)

	// Initialize WorkflowManager
	wm := &WorkflowManager{}

	// Set as global instance
	SetGlobalWorkflowManager(wm)

	// Register tools based on configuration
	err = RegisterTools(wm, config)
	if err != nil {
		log.Printf("Failed to register tools: %v", err)
	}

	// Get the list of tools from the WorkflowManager
	tools := wm.ListTools()
	fmt.Println("Registered Tools:")
	fmt.Println(tools)

	// Declare variables for the completions service
	var completionsService *ExternalService
	var completionsCtx context.Context
	var cancel context.CancelFunc

	switch config.LLMBackend {
	case "gguf":
		llmService := config.Services[1]
		completionsService = NewExternalService(llmService, verbose)
		completionsCtx, cancel = context.WithCancel(context.Background())

		if err := completionsService.Start(completionsCtx); err != nil {
			e.Logger.Fatal(err)
		}

		// Construct the base URL from Host and Port
		baseURL := fmt.Sprintf("http://%s:%d/v1", llmService.Host, llmService.Port)
		llmClient = NewLocalLLMClient(baseURL, "", "")

	case "mlx":
		llmService := config.Services[2]
		completionsService = NewExternalService(llmService, verbose)
		completionsCtx, cancel = context.WithCancel(context.Background())

		if err := completionsService.Start(completionsCtx); err != nil {
			e.Logger.Fatal(err)
		}

		// Construct the base URL from Host and Port
		baseURL := fmt.Sprintf("http://%s:%d/v1", llmService.Host, llmService.Port)
		llmClient = NewLocalLLMClient(baseURL, "", "")

	case "openai":
		completionsCtx, cancel = context.WithCancel(context.Background())
		if config.OpenAIAPIKey == "" {
			log.Fatal("OpenAI API key is not set in config")
		}
		llmClient = NewLocalLLMClient("https://api.openai.com/v1", "gpt-4o-mini", config.OpenAIAPIKey)

	default:
		log.Fatal("Invalid LLMBackend specified in config")
	}

	var embeddingsService *ExternalService

	// Initialize the embeddings service
	embeddingsConfig := config.Services[4]
	embeddingsService = NewExternalService(embeddingsConfig, verbose)
	if err := embeddingsService.Start(completionsCtx); err != nil {
		e.Logger.Fatal(err)
	}

	// Set up graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		// Cancel the context to signal all operations to stop
		if cancel != nil {
			cancel()
		}

		// Stop the completions service first
		if completionsService != nil {
			ctx, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelTimeout()
			if err := completionsService.Stop(ctx); err != nil {
				e.Logger.Info(err)
			}
		}

		// Then shut down the Echo server
		ctx, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelTimeout()
		if err := e.Shutdown(ctx); err != nil {
			e.Logger.Error(err)
		}
	}()

	e.Logger.Info(e.Start(fmt.Sprintf(":%d", config.Services[0].Port)))
}

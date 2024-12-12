// manifold/main.go

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"manifold/internal/documents"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/jaegertracing"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var (
	completionsService *ExternalService
	completionsCtx     context.Context
	cancel             context.CancelFunc
	llmClient          LLMClient
	//searchIndex        bleve.Index
	indexManager *documents.IndexManager
	docManager   *documents.DocumentManager
	db           *SQLiteDB
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

	// Print the config.services with their index and name
	for i, service := range config.Services {
		log.Printf("Service %d: %s", i, service.Name)
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
	config.SelectedModels, _ = GetSelectedModels(db.db)

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

	// Register the enabled tools
	for _, toolConfig := range config.Tools {
		params, _ := db.GetToolMetadataByName(toolConfig.Name)

		if params.Enabled {
			// Create the tool
			tool, err := CreateToolByName(toolConfig.Name)
			if err != nil {
				log.Printf("Failed to create tool '%s': %v", toolConfig.Name, err)
				continue
			}

			// Add the tool to the WorkflowManager
			err = wm.AddTool(tool, toolConfig.Name)
			if err != nil {
				log.Printf("Failed to add tool '%s' to WorkflowManager: %v", toolConfig.Name, err)
			}
		}
	}

	// Get the list of tools from the WorkflowManager
	tools := wm.ListTools()
	fmt.Println("Registered Tools:")
	fmt.Println(tools)

	var embeddingsService *ExternalService
	var embeddingsCtx context.Context

	// Initialize the embeddings service
	embeddingsConfig := config.Services[4]

	// Print the embeddings service configuration
	log.Println("Embeddings service configuration:")
	log.Println(embeddingsConfig)

	embeddingsService = NewExternalService(embeddingsConfig, true)

	// Initialize embeddings context before starting the service
	embeddingsCtx, embeddingsCancel := context.WithCancel(context.Background())
	defer embeddingsCancel()

	if err := embeddingsService.Start(embeddingsCtx); err != nil {
		e.Logger.Fatal(err)
	}

	switch config.LLMBackend {
	case "gguf":
		config.Services[1].Args = []string{
			"--model",
			config.SelectedModels.ModelPath,
			"--port",
			"32182",
			"--host",
			"0.0.0.0",
			"--gpu-layers",
			"99",
		}
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
		// Get the path to the folder containing the model
		mlxModelPath := fmt.Sprintf("%s/models-mlx/%s", config.DataPath, config.SelectedModels.ModelName)

		// Print the selected model path
		log.Println("Selected model path:", mlxModelPath)

		config.Services[2].Args = []string{
			"--model",
			mlxModelPath,
			"--port",
			"32182",
			"--host",
			"0.0.0.0",
			"--log-level",
			"DEBUG",
		}
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
	case "gemini":
		completionsCtx, cancel = context.WithCancel(context.Background())
		if config.GoogleAPIKey == "" {
			log.Fatal("Google API key is not set in config")
		}
		llmClient = NewLocalLLMClient("https://generativelanguage.googleapis.com/v1beta/openai", "gemini-2.0-flash-exp", config.GoogleAPIKey)

	default:
		log.Fatal("Invalid LLMBackend specified in config")
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

// function to restart completions service with new model
func restartCompletionsService(config *Config, verbose bool) {
	if completionsService != nil {
		ctx, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelTimeout()
		if err := completionsService.Stop(ctx); err != nil {
			log.Println(err)
		}
	}

	// Start completions service with new model
	switch config.LLMBackend {
	case "gguf":
		log.Println("Selected model path:", config.SelectedModels.ModelPath)
		config.Services[1].Args = []string{
			"--model",
			config.SelectedModels.ModelPath,
			"--port",
			"32182",
			"--host",
			"0.0.0.0",
			"--gpu-layers",
			"99",
			"--ctx-size",
			"128000",
		}
		llmService := config.Services[1]
		completionsService = NewExternalService(llmService, verbose)
		completionsCtx, cancel = context.WithCancel(context.Background())

		if err := completionsService.Start(completionsCtx); err != nil {
			log.Fatal(err)
		}

		// Construct the base URL from Host and Port
		baseURL := fmt.Sprintf("http://%s:%d/v1", llmService.Host, llmService.Port)
		llmClient = NewLocalLLMClient(baseURL, "", "")

	case "mlx":
		// Get the path to the folder containing the model
		mlxModelPath := fmt.Sprintf("%s/models-mlx/%s", config.DataPath, config.SelectedModels.ModelName)

		// Print the selected model path
		log.Println("Selected model path:", mlxModelPath)

		config.Services[2].Args = []string{
			"--model",
			mlxModelPath,
			"--port",
			"32182",
			"--host",
			"0.0.0.0",
			"--log-level",
			"DEBUG",
		}

		llmService := config.Services[2]
		llmService.Model = config.SelectedModels.ModelPath
		completionsService = NewExternalService(llmService, verbose)
		completionsCtx, cancel = context.WithCancel(context.Background())

		if err := completionsService.Start(completionsCtx); err != nil {
			log.Fatal(err)
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
	case "gemini":
		completionsCtx, cancel = context.WithCancel(context.Background())
		if config.GoogleAPIKey == "" {
			log.Fatal("Google API key is not set in config")
		}
		llmClient = NewLocalLLMClient("https://generativelanguage.googleapis.com/v1beta/openai", "gemini-2.0-flash-exp", config.GoogleAPIKey)

	default:
		log.Fatal("Invalid LLMBackend specified in config")
	}
}

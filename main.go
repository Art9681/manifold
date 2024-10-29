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
	completionsService *ExternalService
	completionsCtx     context.Context
	cancel             context.CancelFunc
	llmClient          LLMClient
	searchIndex        bleve.Index
	db                 *SQLiteDB
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
	config.SelectedModels, _ = GetSelectedModels(db.db)

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
	// var completionsService *ExternalService
	// var completionsCtx context.Context
	// var cancel context.CancelFunc

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
		// args:
		// - --host
		// - 0.0.0.0
		// - --port
		// - 32182
		// - --model
		// - /Users/arturoaquino/.eternal-v1/models-mlx/Qwen2.5-72B-Instruct-8bit
		// - --log-level
		// - DEBUG

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
		// args:
		// - --model
		// - /Users/arturoaquino/.eternal-v1/models-gguf/supernova-medius-14b/SuperNova-Medius-Q8_0.gguf
		// - --port
		// - 32182
		// - --host
		// - 0.0.0.0
		// - --gpu-layers
		// - 99
		// print the selected model path

		config.SelectedModels.ModelPath = "E:\\manifold\\data\\models-gguf\\qwen2.5-32b\\Qwen2.5-32B-Instruct-Q4_K_L.gguf"
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

	default:
		log.Fatal("Invalid LLMBackend specified in config")
	}
}

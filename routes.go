// manifold/routes.go

package main

import (
	"html/template"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// TemplateRenderer is a custom html/template renderer for Echo framework
type TemplateRenderer struct {
	templates *template.Template
}

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func setupRoutes(e *echo.Echo, config *Config) {
	t := &TemplateRenderer{
		templates: template.Must(template.ParseGlob("public/views/*.html")),
	}

	e.Renderer = t
	e.Static("/", "public")

	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "base.html", nil)
	})

	e.GET("/v1/config", func(c echo.Context) error {
		return handleGetConfig(c, config)
	})

	// chat submit route
	e.POST("/v1/chat/submit", handleChatSubmit)
	e.POST("/v1/chat/role/:role", func(c echo.Context) error {
		return handleSetChatRole(c, config)
	})

	// model routes
	e.POST("/v1/models/select", func(c echo.Context) error {
		modelName := c.FormValue("modelName")
		if modelName == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Model name is required"})
		}

		err := SetSelectedModel(db.db, modelName)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to set selected model"})
		}

		// update the config
		config.SelectedModels, _ = GetSelectedModels(db.db)

		// restart the completions service
		restartCompletionsService(config, true)

		// Return json object with status and model name
		return c.JSON(http.StatusOK, map[string]string{"status": "success", "model": modelName})
	})

	// Tool routes
	e.POST("/v1/tools/:toolName/toggle", func(c echo.Context) error {
		return handleToolToggle(c, config)
	})
	e.GET("/v1/tools/list", handleGetTools)

	// Retrieval Augmented Generation (RAG) routes
	// Route for storing text and embeddings
	e.POST("/v1/embeddings", handleEmbeddingRequest)

	// Document routes
	e.POST("/v1/documents/ingest/git", handleGitIngest)
	e.POST("/v1/documents/ingest/pdf", handlePDFIngest)
	e.POST("/v1/documents/split", handleSplitDocuments)
	e.POST("/v1/documents/query", func(c echo.Context) error {
		err := handleQueryDocuments(c)
		return err
	})

	// tool routes
	//e.GET("/v1/tools", handleRenderTools)

	e.GET("/ws", handleWebSocketConnection)
}

// handleGetConfig is a handler for getting the configuration
func handleGetConfig(c echo.Context, config *Config) error {
	return c.JSON(http.StatusOK, config)
}

// Helper function to convert bool to string
func boolToString(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

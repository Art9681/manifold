// routes.go

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

	// tool routes
	//e.GET("/v1/tools", handleRenderTools)

	e.GET("/ws", handleWebSocketConnection)
}

// handleGetConfig is a handler for getting the configuration
func handleGetConfig(c echo.Context, config *Config) error {
	return c.JSON(http.StatusOK, config)
}

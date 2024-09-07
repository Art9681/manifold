// manifold/config.go

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type ServiceConfig struct {
	Name      string   `yaml:"name"`
	Host      string   `yaml:"host"`
	Port      int      `yaml:"port"`
	Command   string   `yaml:"command"`
	GPULayers string   `yaml:"gpu_layers,omitempty"`
	Args      []string `yaml:"args,omitempty"`
	Model     string   `yaml:"model,omitempty"`
}

type Config struct {
	DataPath       string            `yaml:"data_path,omitempty"`
	LLMBackend     string            `yaml:"llm_backend"`
	Services       []ServiceConfig   `yaml:"services"`
	Tools          []YAMLTool        `yaml:"tools"`
	LanguageModels []ModelParams     `yaml:"language_models"`
	Roles          []CompletionsRole `yaml:"roles"`
}

type Tool struct {
	ID         uint   `gorm:"primaryKey"`
	Name       string `gorm:"unique;not null"`
	Enabled    bool
	Parameters []ToolParam `gorm:"foreignKey:ToolID"`
}

type ToolParam struct {
	ID         uint `gorm:"primaryKey"`
	ToolID     uint
	ParamName  string
	ParamValue string
}

type YAMLTool struct {
	Name       string                 `yaml:"name"`
	Enabled    bool                   `yaml:"enabled"`
	Parameters map[string]interface{} `yaml:",inline"`
}

type WebGetTool struct {
	Tool
	TopN int `yaml:"top_n"`
}

type WebSearchTool struct {
	Tool
	Endpoint string `yaml:"endpoint,omitempty"`
	TopN     int    `yaml:"top_n"`
}

type ImgGenConfig struct {
	Workflow string
	Width    int
	Height   int
}

type MemoryTool struct {
	Tool
	TopN int `yaml:"top_n"`
}

type TeamTool struct {
	Tool
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Set default values for each Language model
	for i := range config.LanguageModels {
		if config.LanguageModels[i].Temperature == 0.0 {
			config.LanguageModels[i].Temperature = 0.7
		}
		if config.LanguageModels[i].TopP == 0.0 {
			config.LanguageModels[i].TopP = 0.9
		}
		if config.LanguageModels[i].TopK == 0 {
			config.LanguageModels[i].TopK = 95
		}
		if config.LanguageModels[i].RepetitionPenalty == 0.0 {
			config.LanguageModels[i].RepetitionPenalty = 1.0
		}
	}

	return &config, nil
}

func (c *Config) GetMLXServiceConfig() (*ServiceConfig, error) {
	for _, service := range c.Services {
		if service.Name == "mlx_server" {
			return &service, nil
		}
	}
	return nil, fmt.Errorf("MLX service configuration not found")
}

// handleGetConfig is a handler for getting the configuration
// func handleGetConfig(c echo.Context, config *Config, db *SQLiteDB) error {
// 	// Retrieve the models from the database
// 	models := []ModelParams{}
// 	if err := db.Find(&models).Error; err != nil {
// 		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve models from the database"})
// 	}

// 	config.LanguageModels = models

// 	return c.JSON(http.StatusOK, config)
// }

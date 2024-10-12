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

type ToolConfig struct {
	Name       string                 `yaml:"name"`
	Parameters map[string]interface{} `yaml:"parameters"`
}

type Config struct {
	OpenAIAPIKey   string            `yaml:"openai_api_key,omitempty"`
	DataPath       string            `yaml:"data_path"`
	LLMBackend     string            `yaml:"llm_backend"`
	Services       []ServiceConfig   `yaml:"services"`
	Tools          []ToolConfig      `yaml:"tools"`
	Roles          []CompletionsRole `yaml:"roles"`
	LanguageModels []LanguageModel   `json:"language_models"`
	SelectedModels SelectedModels    `json:"selected_models"`
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

	return &config, nil
}

// func LoadConfig(filename string) (*Config, error) {
// 	data, err := os.ReadFile(filename)
// 	if err != nil {
// 		return nil, fmt.Errorf("error reading config file: %w", err)
// 	}

// 	var config Config
// 	err = yaml.Unmarshal(data, &config)
// 	if err != nil {
// 		return nil, fmt.Errorf("error unmarshaling config: %w", err)
// 	}

// 	// Ensure OpenAI API key is set if backend is OpenAI
// 	if config.LLMBackend == "openai" && config.OpenAIAPIKey == "" {
// 		return nil, fmt.Errorf("openai_api_key must be set when llm_backend is 'openai'")
// 	} else {
// 		// Set default values for each Language model
// 		for i := range config.LanguageModels {
// 			if config.LanguageModels[i].Temperature == 0.0 {
// 				config.LanguageModels[i].Temperature = 0.7
// 			}
// 			if config.LanguageModels[i].TopP == 0.0 {
// 				config.LanguageModels[i].TopP = 0.9
// 			}
// 			if config.LanguageModels[i].TopK == 0 {
// 				config.LanguageModels[i].TopK = 95
// 			}
// 			if config.LanguageModels[i].RepetitionPenalty == 0.0 {
// 				config.LanguageModels[i].RepetitionPenalty = 1.0
// 			}
// 		}
// 	}

// 	return &config, nil
// }

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
// 	models := []LanguageModels{}
// 	if err := db.Find(&models).Error; err != nil {
// 		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve models from the database"})
// 	}

// 	config.LanguageModels = models

// 	return c.JSON(http.StatusOK, config)
// }

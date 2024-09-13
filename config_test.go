// config_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigValid(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")
	configContent := `
openai_api_key: "test-api-key"
llm_backend: "openai"
services:
  - name: "service1"
    host: "localhost"
    port: 8080
tools:
  - name: "WebGetTool"
    enabled: true
    params:
      - param_name: "TopN"
        param_value: "5"
language_models:
  - name: "gpt-4"
    homepage: "https://openai.com/gpt-4"
    downloads: "https://download.openai.com/gpt-4"
    temperature: 0.7
    top_p: 0.9
    top_k: 95
    repetition_penalty: 1.0
    prompt: "Default prompt"
    ctx: 2048
    downloaded: false
roles:
  - name: "admin"
    instructions: "Admin role instructions."
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to write temporary config file")

	config, err := LoadConfig(configPath)
	require.NoError(t, err, "LoadConfig should not return an error for valid config")

	assert.Equal(t, "test-api-key", config.OpenAIAPIKey)
	assert.Equal(t, "openai", config.LLMBackend)
	assert.Equal(t, 1, len(config.Services))
	assert.Equal(t, "service1", config.Services[0].Name)
	assert.Equal(t, "localhost", config.Services[0].Host)
	assert.Equal(t, 8080, config.Services[0].Port)
	assert.Equal(t, 1, len(config.Tools))
	assert.Equal(t, "WebGetTool", config.Tools[0].Name)
	assert.True(t, config.Tools[0].Enabled)
	assert.Equal(t, 1, len(config.Tools[0].Params))
	assert.Equal(t, "TopN", config.Tools[0].Params[0].ParamName)
	assert.Equal(t, "5", config.Tools[0].Params[0].ParamValue)
	assert.Equal(t, 1, len(config.LanguageModels))
	assert.Equal(t, "gpt-4", config.LanguageModels[0].Name)
	assert.Equal(t, "https://openai.com/gpt-4", config.LanguageModels[0].Homepage)
	assert.Equal(t, "https://download.openai.com/gpt-4", config.LanguageModels[0].Downloads)
	assert.Equal(t, 0.7, config.LanguageModels[0].Temperature)
	assert.Equal(t, 0.9, config.LanguageModels[0].TopP)
	assert.Equal(t, 95, config.LanguageModels[0].TopK)
	assert.Equal(t, 1.0, config.LanguageModels[0].RepetitionPenalty)
	assert.Equal(t, "Default prompt", config.LanguageModels[0].Prompt)
	assert.Equal(t, 2048, config.LanguageModels[0].Ctx)
	assert.False(t, config.LanguageModels[0].Downloaded)
	assert.Equal(t, 1, len(config.Roles))
	assert.Equal(t, "admin", config.Roles[0].Name)
	assert.Equal(t, "Admin role instructions.", config.Roles[0].Instructions)
}

func TestLoadConfigMissingAPIKey(t *testing.T) {
	// Create a temporary config file without OpenAIAPIKey
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")
	configContent := `
llm_backend: "openai"
services:
  - name: "service1"
    host: "localhost"
    port: 8080
tools:
  - name: "WebGetTool"
    enabled: true
    params:
      - param_name: "TopN"
        param_value: "5"
language_models:
  - name: "gpt-4"
    homepage: "https://openai.com/gpt-4"
    downloads: "https://download.openai.com/gpt-4"
    temperature: 0.7
    top_p: 0.9
    top_k: 95
    repetition_penalty: 1.0
    prompt: "Default prompt"
    ctx: 2048
    downloaded: false
roles:
  - name: "admin"
    instructions: "Admin role instructions."
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to write temporary config file")

	_, err = LoadConfig(configPath)
	require.Error(t, err, "LoadConfig should return an error when OpenAIAPIKey is missing")
	assert.Contains(t, err.Error(), "openai_api_key must be set when llm_backend is 'openai'")
}

func TestLoadConfigDefaultValues(t *testing.T) {
	// Create a temporary config file without some optional fields
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")
	configContent := `
llm_backend: "mlx"
services:
  - name: "service1"
    host: "localhost"
    port: 8080
  - name: "llm_service"
    host: "llm.local"
    port: 9090
tools:
  - name: "WebGetTool"
    enabled: true
    params:
      - param_name: "TopN"
        param_value: "10"
language_models:
  - name: "gpt-3"
    homepage: "https://openai.com/gpt-3"
    downloads: "https://download.openai.com/gpt-3"
    prompt: "Default prompt for GPT-3"
    ctx: 1024
    downloaded: true
roles:
  - name: "user"
    instructions: "User role instructions."
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to write temporary config file")

	config, err := LoadConfig(configPath)
	require.NoError(t, err, "LoadConfig should not return an error for valid config")

	// Check default values
	for i := range config.LanguageModels {
		if config.LanguageModels[i].Temperature == 0.0 {
			assert.Equal(t, 0.7, config.LanguageModels[i].Temperature)
		}
		if config.LanguageModels[i].TopP == 0.0 {
			assert.Equal(t, 0.9, config.LanguageModels[i].TopP)
		}
		if config.LanguageModels[i].TopK == 0 {
			assert.Equal(t, 95, config.LanguageModels[i].TopK)
		}
		if config.LanguageModels[i].RepetitionPenalty == 0.0 {
			assert.Equal(t, 1.0, config.LanguageModels[i].RepetitionPenalty)
		}
	}
}

package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tempFile, err := os.CreateTemp("", "config*.yml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	// Write test configuration
	testConfig := `
services:
  - name: mlx_server
    host: localhost
    port: 8080
    command: test_command
    args:
      - arg1
      - arg2
language_models:
  - name: test_model
    path: /path/to/model
`
	if _, err := tempFile.Write([]byte(testConfig)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	// Test loading the config
	config, err := LoadConfig(tempFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify the loaded config
	if len(config.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]
	if service.Name != "mlx_server" {
		t.Errorf("Expected service name 'mlx_server', got '%s'", service.Name)
	}
	if service.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", service.Host)
	}
	if service.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", service.Port)
	}
	if service.Command != "test_command" {
		t.Errorf("Expected command 'test_command', got '%s'", service.Command)
	}
	if len(service.Args) != 2 || service.Args[0] != "arg1" || service.Args[1] != "arg2" {
		t.Errorf("Unexpected args: %v", service.Args)
	}

	// Verify the language models
	if len(config.LanguageModels) != 1 {
		t.Errorf("Expected 1 language model, got %d", len(config.LanguageModels))
	}

	model := config.LanguageModels[0]
	if model.Name != "test_model" {
		t.Errorf("Expected model name 'test_model', got '%s'", model.Name)
	}
}

func TestGetMLXServiceConfig(t *testing.T) {
	config := &Config{
		Services: []ServiceConfig{
			{Name: "other_service"},
			{Name: "mlx_server", Host: "localhost", Port: 8080},
		},
	}

	mlxConfig, err := config.GetMLXServiceConfig()
	if err != nil {
		t.Fatalf("GetMLXServiceConfig failed: %v", err)
	}

	if mlxConfig.Name != "mlx_server" {
		t.Errorf("Expected service name 'mlx_server', got '%s'", mlxConfig.Name)
	}
	if mlxConfig.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", mlxConfig.Host)
	}
	if mlxConfig.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", mlxConfig.Port)
	}
}

func TestExternalService(t *testing.T) {
	config := ServiceConfig{
		Name:    "test_service",
		Command: "sleep",
		Args:    []string{"1"},
	}

	service := NewExternalService(config, false)

	// Test starting the service
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// Test stopping the service
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := service.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop service: %v", err)
	}
}

func TestHostInfoProvider(t *testing.T) {
	host := NewHostInfoProvider()

	if host.GetOS() == "" {
		t.Error("GetOS returned empty string")
	}
	if host.GetArch() == "" {
		t.Error("GetArch returned empty string")
	}
	if host.GetCPUs() <= 0 {
		t.Error("GetCPUs returned non-positive value")
	}
	if host.GetMemory() <= 0 {
		t.Error("GetMemory returned non-positive value")
	}

	gpus, err := host.GetGPUs()
	if err != nil {
		t.Fatalf("GetGPUs failed: %v", err)
	}
	if len(gpus) == 0 {
		t.Log("No GPUs detected, this may be normal depending on the system")
	} else {
		for i, gpu := range gpus {
			if gpu.GetModel() == "" {
				t.Errorf("GPU #%d has empty model", i+1)
			}
		}
	}
}

func TestInitializeApplication(t *testing.T) {
	config := &Config{
		DataPath: "/tmp/test_manifold",
	}

	db, _, err := initializeApplication(config)
	if err != nil {
		t.Fatalf("initializeApplication failed: %v", err)
	}

	err = db.AutoMigrate(ModelParams{}, URLTracking{})
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	model := ModelParams{Name: "test_model"}
	err = db.Create(&model)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
}

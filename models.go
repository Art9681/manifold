// manifold/models.go

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ModelManager manages AI models stored locally and handles downloading models from URLs.
type ModelManager struct {
	modelsDir string
}

// NewModelManager creates a new ModelManager with the given models directory.
func NewModelManager(modelsDir string) *ModelManager {
	return &ModelManager{
		modelsDir: modelsDir,
	}
}

// ScanModels scans the models directory recursively and returns a list of model paths.
func (mm *ModelManager) ScanModels() ([]string, error) {
	var modelPaths []string

	// Walk through the directory recursively
	err := filepath.Walk(mm.modelsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Add files that match common model file extensions
		if strings.HasSuffix(strings.ToLower(info.Name()), ".onnx") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".safetensors") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".ckpt") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".bin") {
			modelPaths = append(modelPaths, path)

			// If the model is split into multiple files, add the first file only
			if strings.HasSuffix(strings.ToLower(info.Name()), ".safetensors") || strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
				return filepath.SkipDir
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan models directory: %w", err)
	}

	return modelPaths, nil
}

// DownloadModel downloads a model from the specified URL and saves it to the models directory.
// It returns the local path to the downloaded model.
func (mm *ModelManager) DownloadModel(url string) (string, error) {
	// Extract the file name from the URL
	tokens := strings.Split(url, "/")
	fileName := tokens[len(tokens)-1]
	localPath := filepath.Join(mm.modelsDir, fileName)

	// Create the file
	out, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Download the file
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download model, status code: %d", resp.StatusCode)
	}

	// Write the content to the local file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save model: %w", err)
	}

	log.Printf("Model downloaded and saved to: %s", localPath)
	return localPath, nil
}

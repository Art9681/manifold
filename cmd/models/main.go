package main

import (
	"log"
)

func main() {
	modelsDir := "/Users/arturoaquino/.eternal-v1" // Update this path as needed
	mm := NewModelManager(modelsDir)

	// Scan for models
	models, err := mm.ScanModels()
	if err != nil {
		log.Fatalf("Error scanning models: %v", err)
	}

	for _, model := range models {
		log.Println(model)
	}

	// Download a model
	// modelURL := "https://example.com/path/to/your/model.onnx" // Replace with an actual URL
	// downloadedPath, err := mm.DownloadModel(modelURL)
	// if err != nil {
	// 	log.Fatalf("Error downloading model: %v", err)
	// }

	// log.Printf("Model downloaded to: %s", downloadedPath)
}

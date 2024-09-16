package embeddings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

// Define an interface that includes the methods we need from AppConfig
type ConfigProvider struct {
	DataPath string
}

// Embedding represents the structure of the JSON output
type GgufEmbedding struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func GgufEmbed(prompt string, config *ConfigProvider) []float64 {
	cmdpath := fmt.Sprintf("%s/gguf/llama-embedding", config.DataPath)
	modelpath := fmt.Sprintf("%s/models/e5-mistral-embed/e5-mistral-7b-instruct-Q8_0.gguf", config.DataPath)

	cmd := exec.Command(
		cmdpath,
		"--no-display-prompt",
		"-ngl", "99",
		"-m", modelpath,
		"-p", prompt,
		"--pooling", "last",
		"--embd-normalize", "2", //normalisation for embendings (default: 2) (-1=none, 0=max absolute int16, 1=taxicab, 2=euclidean, >2=p-norm)
		"--embd-output-format", "json+",
	)

	// Create a buffer to capture the output
	var out bytes.Buffer
	cmd.Stdout = &out

	// Run the command and check for errors
	err := cmd.Run()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}

	// Parse the JSON output
	var embedding GgufEmbedding
	err = json.Unmarshal(out.Bytes(), &embedding)
	if err != nil {
		log.Fatalf("json.Unmarshal() failed with %s\n", err)
	}

	// Print the parsed output
	// log.Printf("Parsed Embedding: %+v\n", embedding.Data[0].Embedding)

	return embedding.Data[0].Embedding
}

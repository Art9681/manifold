package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

type EmbeddingResponse struct {
	Object string       `json:"object"`
	Data   []Embedding  `json:"data"`
	Model  string       `json:"model"`
	Usage  UsageMetrics `json:"usage"`
}

type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

func handleEmbeddingRequest(c echo.Context) error {
	// var request struct {
	// 	Input string `json:"input"`
	// }

	// Assuming you have a LLM client set up like in completions
	var embeddingRequest EmbeddingRequest

	if err := c.Bind(&embeddingRequest); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// Send the request to the LLM
	resp, err := llmClient.SendEmbeddingRequest(&embeddingRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get embeddings"})
	}
	defer resp.Body.Close()

	fmt.Printf("Response: %v\n", resp)

	var embeddingResponse EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to decode embeddings"})
	}

	return c.JSON(http.StatusOK, embeddingResponse)
}

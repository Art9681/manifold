package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

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

// Define the request structure for storing documents
type StoreDocumentRequest struct {
	Text string `json:"text"`
}

// Handler to store text and embeddings in Badger
func handleStoreDocument(c echo.Context) error {
	// Parse the request body
	req := new(StoreDocumentRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
	}

	// Validate input
	if req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Text cannot be empty")
	}

	// Generate embeddings for the input text
	embedding, err := GenerateEmbedding(req.Text)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate embeddings")
	}

	// Create a unique document ID
	docID := fmt.Sprintf("doc-%d", time.Now().UnixNano())

	// Find the RetrievalTool in the WorkflowManager to store the document
	var retrievalTool *RetrievalTool
	for _, tool := range globalWM.tools {
		if tool.Name == "retrieval" {
			retrievalTool, _ = tool.Tool.(*RetrievalTool)
			break
		}
	}

	if retrievalTool == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Retrieval tool not found")
	}

	// Store the document and embeddings in Badger
	err = retrievalTool.StoreDocument(docID, req.Text, embedding)
	if err != nil {
		log.Printf("Error storing document in Badger: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to store document")
	}

	// Return success response
	return c.JSON(http.StatusOK, map[string]string{
		"message":  "Document stored successfully",
		"document": docID,
	})
}

// tool.go

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"manifold/internal/web"

	"github.com/labstack/echo/v4"
)

// Tool interface defines the contract for all tools.
type Tool interface {
	Process(ctx context.Context, input string) (string, error)
	Enabled() bool
	SetParams(params map[string]interface{}) error
}

// ToolWrapper wraps a Tool with its name for management.
type ToolWrapper struct {
	Tool Tool
	Name string
}

// WorkflowManager manages a set of tools and runs them in sequence.
type WorkflowManager struct {
	tools []ToolWrapper
}

// RegisterTools initializes and registers all enabled tools based on the configuration.
func RegisterTools(wm *WorkflowManager, config *Config) error {
	for _, toolConfig := range config.Tools {
		// Print tool configuration for debugging
		log.Printf("Tool: %s, Parameters: %v", toolConfig.Name, toolConfig.Parameters)

		switch toolConfig.Name {
		case "websearch":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				tool := &WebSearchTool{}
				err := tool.SetParams(toolConfig.Parameters)
				if err != nil {
					return fmt.Errorf("failed to set params for tool %s: %w", toolConfig.Name, err)
				}
				wm.AddTool(tool, toolConfig.Name)
			}
		case "webget":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				tool := &WebGetTool{}
				err := tool.SetParams(toolConfig.Parameters)
				if err != nil {
					return fmt.Errorf("failed to set params for tool %s: %w", toolConfig.Name, err)
				}
				wm.AddTool(tool, toolConfig.Name)
			}
		case "retrieval":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				tool := &RetrievalTool{}
				err := tool.SetParams(toolConfig.Parameters)
				if err != nil {
					return fmt.Errorf("failed to set params for tool %s: %w", toolConfig.Name, err)
				}
				wm.AddTool(tool, toolConfig.Name)
			}
		}
	}

	return nil
}

// AddTool adds a new tool to the workflow if it is enabled.
func (wm *WorkflowManager) AddTool(tool Tool, name string) error {
	wm.tools = append(wm.tools, ToolWrapper{Tool: tool, Name: name})
	return nil
}

// RemoveTool removes a tool from the workflow by name.
func (wm *WorkflowManager) RemoveTool(name string) error {
	for i, wrapper := range wm.tools {
		if wrapper.Name == name {
			wm.tools = append(wm.tools[:i], wm.tools[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("tool %s not found", name)
}

// ListTools returns a list of tool names in the workflow.
func (wm *WorkflowManager) ListTools() []string {
	var toolNames []string
	for _, wrapper := range wm.tools {
		toolNames = append(toolNames, wrapper.Name)
	}
	return toolNames
}

// Run executes all enabled tools in the workflow sequentially.
func (wm *WorkflowManager) Run(ctx context.Context, prompt string) (string, error) {
	// If no tools are enabled, return the prompt as is
	if len(wm.tools) == 0 {
		return prompt, nil
	}

	// Get the list of enabled tools and print their names
	log.Printf("Enabled tools: %v", wm.ListTools())

	var allContent strings.Builder

	for _, wrapper := range wm.tools {
		processed, err := wrapper.Tool.Process(ctx, prompt)
		if err != nil {
			log.Printf("error processing with tool %s: %v", wrapper.Name, err)
		}

		// Append the processed output to the final content
		allContent.WriteString(processed)
	}

	retrievalInstructions := `\n\nYou have been provided with relevant document chunks retrieved from a retrieval-augmented generation (RAG) workflow. Use the information contained in these chunks to assist in generating your response only if it directly contributes to answering the user's prompt. You must ensure that:

	You do not explicitly reference or mention the existence of these chunks.
	You seamlessly incorporate relevant information into your response as if it were part of your own knowledge.
	If the provided chunks are not helpful for addressing the user's prompt, you may generate a response based on your general knowledge.
	Now proceed with answering the user's prompt:\n\n`

	// Append the retrieval instructions to the final content
	allContent.WriteString(retrievalInstructions)

	// Append the prompt to the final content
	allContent.WriteString(prompt)

	return allContent.String(), nil
}

// WebSearchTool is an existing tool for performing web searches.
type WebSearchTool struct {
	enabled      bool
	SearchEngine string
	Endpoint     string
	TopN         int
	Concurrency  int // New field to control concurrency
}

// Process executes the web search tool logic.
func (t *WebSearchTool) Process(ctx context.Context, input string) (string, error) {
	// Print the search engine and endpoint for debugging
	log.Printf("Search Engine: %s, Endpoint: %s", t.SearchEngine, t.Endpoint)

	// Perform search using GetSearXNGResults
	urls := web.GetSearXNGResults(t.Endpoint, input)

	if len(urls) == 0 {
		return "", errors.New("no URLs found after filtering")
	} else {
		// Only return the topN URLs
		if t.TopN > len(urls) {
			t.TopN = len(urls)
		}
		urls = urls[:t.TopN]
	}

	// Print the URLs for debugging
	log.Printf("URLs: %v", urls)

	// Fetch contents concurrently
	type result struct {
		content string
		err     error
	}

	resultsChan := make(chan result, t.TopN)
	var wg sync.WaitGroup

	// Semaphore to limit concurrency
	semaphore := make(chan struct{}, t.Concurrency)

	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			content, err := web.WebGetHandler(ctx, url)
			if err != nil {
				log.Printf("Failed to fetch content from URL %s: %v", url, err)
				resultsChan <- result{content: "", err: err}
				//return
			}

			// If the content does not start with 'I do not have access to real-time information, including news updates.', save it
			if !strings.HasPrefix(content, "I do not have access to real-time information, including news updates.") {
				err = SaveChatTurn(input, content, time.Now().Format(time.RFC3339))
				if err != nil {
					log.Printf("Failed to save web document: %v", err)
				}
			}

			resultsChan <- result{content: content, err: nil}
		}(u)
	}

	// Wait for all fetches to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// var aggregatedContent strings.Builder

	// for res := range resultsChan {
	// 	if res.err == nil && res.content != "" {
	// 		aggregatedContent.WriteString(res.content)
	// 		aggregatedContent.WriteString("\n") // Separator between contents
	// 	}
	// }

	// finalResult := aggregatedContent.String()
	// if finalResult == "" {
	// 	return input, errors.New("no tools processed the input or no result generated")
	// }

	return "", nil
}

// Enabled returns the enabled status of the tool.
func (t *WebSearchTool) Enabled() bool {
	return t.enabled
}

// SetParams configures the tool with provided parameters.
func (t *WebSearchTool) SetParams(params map[string]interface{}) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	if searchEngine, ok := params["search_engine"].(string); ok {
		t.SearchEngine = searchEngine
	}
	if endpoint, ok := params["endpoint"].(string); ok {
		t.Endpoint = endpoint
	}
	if topN, ok := params["top_n"].(int); ok {
		t.TopN = topN
	}
	if concurrency, ok := params["concurrency"].(int); ok {
		t.Concurrency = concurrency
	} else {
		t.Concurrency = 5 // Default concurrency level
	}
	return nil
}

// WebGetTool is a new tool for fetching and processing HTML content from URLs in the prompt.
type WebGetTool struct {
	enabled bool
	// Additional parameters can be added here if needed
}

// Process parses URLs from the input, fetches their HTML content, and extracts relevant information.
func (t *WebGetTool) Process(ctx context.Context, input string) (string, error) {
	// Extract URLs from the input using internal/web's ExtractURLs function
	urls := web.ExtractURLs(input)

	if len(urls) == 0 {
		return "", nil // No URLs to process
	}

	// Remove unwanted URLs
	urls = web.RemoveUnwantedURLs(urls)

	var aggregatedContent strings.Builder
	for _, u := range urls {
		// Fetch and process content using internal/web's WebGetHandler function
		content, err := web.WebGetHandler(ctx, u)
		if err != nil {
			log.Printf("Failed to fetch content from URL %s: %v", u, err)
			continue
		}

		aggregatedContent.WriteString(content)
		aggregatedContent.WriteString("\n") // Separator between contents
	}

	timestamp := time.Now().Format(time.RFC3339)

	err := SaveChatTurn(input, aggregatedContent.String(), timestamp)
	if err != nil {
		log.Printf("Failed to save web document: %v", err)
	}

	return aggregatedContent.String(), nil
}

// Enabled returns the enabled status of the tool.
func (t *WebGetTool) Enabled() bool {
	return t.enabled
}

// SetParams configures the tool with provided parameters.
func (t *WebGetTool) SetParams(params map[string]interface{}) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	// Additional parameters can be set here if needed
	return nil
}

type RetrievalTool struct {
	enabled bool
	topN    int
}

// SetParams configures the tool with provided parameters.
func (t *RetrievalTool) SetParams(params map[string]interface{}) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	if topN, ok := params["top_n"].(int); ok {
		t.topN = topN
	} else {
		t.topN = 3 // Default value
	}

	return nil
}

// Process is the main method that processes the input using sqlite fts5
func (t *RetrievalTool) Process(ctx context.Context, input string) (string, error) {
	// Try to retrieve similar documents based on the input embedding
	documents, err := db.RetrieveTopNDocuments(ctx, input, 20)
	if err != nil {
		return "", err
	}

	// Print the retrieved documents for debugging
	log.Printf("Retrieved Documents: %v", documents)

	// Combine the documents into a single string
	var result strings.Builder
	for _, doc := range documents {
		result.WriteString(doc)
		result.WriteString("\n") // Separator between documents
	}

	return result.String(), nil
}

// Enabled returns the enabled status of the tool.
func (t *RetrievalTool) Enabled() bool {
	return t.enabled
}

// CreateToolByName is a helper function to create a tool by its name
func CreateToolByName(toolName string) (Tool, error) {
	switch toolName {
	case "websearch":
		return &WebSearchTool{}, nil
	case "webget":
		return &WebGetTool{}, nil
	case "retrieval":
		return &RetrievalTool{}, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func SaveChatTurn(prompt, response, timestamp string) error {
	// Concatenate the prompt and response
	concatenatedText := fmt.Sprintf("User: %s\nAssistant: %s", prompt, response)

	// Generate embeddings for the prompt and response
	embeddings, err := GenerateEmbedding(concatenatedText)
	if err != nil {
		log.Printf("Error generating embeddings: %v", err)
		return err
	}

	// Convert embeddings to BLOB
	embeddingBlob := embeddingToBlob(embeddings)

	// Insert the prompt, response, and embeddings into the Chat table
	chat := Chat{
		Prompt:    prompt,
		Response:  response,
		ModelName: "assistant",   // Update with actual model name
		Embedding: embeddingBlob, // Store the embedding as BLOB
	}

	// Insert chat into the Chat table
	if err := db.Create(&chat); err != nil {
		return fmt.Errorf("failed to save chat turn: %w", err)
	}

	// Insert the prompt and response into the chat_fts table for full-text search
	if err := db.db.Exec(`
        INSERT INTO chat_fts (prompt, response, modelName) 
        VALUES (?, ?, ?)
    `, prompt, response, "assistant").Error; err != nil {
		return fmt.Errorf("failed to save chat turn in FTS5 table: %w", err)
	}

	return nil
}

func GenerateEmbedding(text string) ([]float64, error) {
	// Invoke the embeddings API
	textArr := []string{text}
	embeddingRequest := EmbeddingRequest{
		Input:          textArr,
		Model:          "assistant",
		EncodingFormat: "float",
	}

	resp, err := llmClient.SendEmbeddingRequest(&embeddingRequest)
	if err != nil {
		log.Printf("Error sending embedding request: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var embeddingResponse EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		log.Printf("Error decoding embedding response: %v", err)
		return nil, err
	}

	if len(embeddingResponse.Data) == 0 {
		return nil, fmt.Errorf("no embeddings found")
	}

	// Concatenate the embeddings into a single slice
	var embeddings []float64
	for _, emb := range embeddingResponse.Data {
		embeddings = append(embeddings, emb.Embedding...)
	}

	return embeddings, nil
}

// handleToolToggle handles the enabling or disabling of a tool.
// It expects a JSON payload with the "enabled" field.
func handleToolToggle(c echo.Context) error {
	// Extract the toolName from the URL parameter
	toolName := c.Param("toolName")

	// Define a struct to parse the incoming JSON payload
	var requestPayload struct {
		Enabled bool `json:"enabled"`
	}

	// Bind the JSON payload to the struct
	if err := c.Bind(&requestPayload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request payload",
		})
	}

	// Fetch the tool metadata from the database
	tool, err := db.GetToolMetadataByName(toolName)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Tool '%s' not found", toolName),
		})
	}

	// Check if the tool is already in the requested state
	if tool.Enabled == requestPayload.Enabled {
		return c.JSON(http.StatusOK, map[string]string{
			"message": fmt.Sprintf("Tool '%s' is already %s", toolName, map[bool]string{true: "enabled", false: "disabled"}[tool.Enabled]),
		})
	}

	log.Printf("Toggling tool '%s' to %s", toolName, map[bool]string{true: "enabled", false: "disabled"}[requestPayload.Enabled])

	// Update the tool's enabled status in the database
	if err := db.UpdateToolMetadataByName(tool.Name, requestPayload.Enabled); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to update tool '%s' status", toolName),
		})
	}

	// Update the WorkflowManager based on the new status
	UpdateWorkflowManagerForToolToggle(toolName, requestPayload.Enabled)

	// Respond with a success message
	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Tool '%s' has been %s", toolName, map[bool]string{true: "enabled", false: "disabled"}[requestPayload.Enabled]),
	})
}

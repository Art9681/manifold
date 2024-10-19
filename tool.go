// manifold/tool.go

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

	index "github.com/blevesearch/bleve_index_api"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// Tool interface defines the contract for all tools.
type Tool interface {
	Process(ctx context.Context, input string) (string, error)
	Enabled() bool
	SetParams(params map[string]interface{}, config *Config) error
	GetParams() map[string]interface{}
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
				err := tool.SetParams(toolConfig.Parameters, config)
				if err != nil {
					return fmt.Errorf("failed to set params for tool %s: %w", toolConfig.Name, err)
				}
				wm.AddTool(tool, toolConfig.Name)
			}
		case "webget":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				tool := &WebGetTool{}
				err := tool.SetParams(toolConfig.Parameters, config)
				if err != nil {
					return fmt.Errorf("failed to set params for tool %s: %w", toolConfig.Name, err)
				}
				wm.AddTool(tool, toolConfig.Name)
			}
		case "retrieval":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				tool := &RetrievalTool{}
				err := tool.SetParams(toolConfig.Parameters, config)
				if err != nil {
					return fmt.Errorf("failed to set params for tool %s: %w", toolConfig.Name, err)
				}
				wm.AddTool(tool, toolConfig.Name)
			}
		case "teams":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				teamServiceConfig := config.Services[5]

				// Print the service configuration for debugging
				log.Printf("Teams Service Config: %v", teamServiceConfig)

				// Prepare parameters including service configuration
				teamsParams := map[string]interface{}{
					"enabled": enabled,
					"service_config": map[string]interface{}{
						"name":    teamServiceConfig.Name,
						"host":    teamServiceConfig.Host,
						"port":    teamServiceConfig.Port,
						"command": teamServiceConfig.Command,
						"args":    teamServiceConfig.Args,
					},
				}

				tool := &TeamsTool{}
				err := tool.SetParams(teamsParams, config)
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
func (wm *WorkflowManager) Run(ctx context.Context, prompt string, c *websocket.Conn) (string, error) {
	// If no tools are enabled, return the prompt as is
	if len(wm.tools) == 0 {
		return prompt, nil
	}

	// Get the list of enabled tools and print their names
	log.Printf("Enabled tools: %v", wm.ListTools())

	var allContent strings.Builder
	var teamsResponse string

	for _, wrapper := range wm.tools {
		var toolMessage string

		switch wrapper.Name {
		case "websearch":
			toolMessage = "Searching the web"
		case "webget":
			toolMessage = "Fetching web content"
		case "retrieval":
			toolMessage = "Trying to remember things"
		case "teams":
			toolMessage = "Asking the team"
		}

		formattedContent := fmt.Sprintf("<div id='progress' class='progress-bar placeholder-wave fs-5' style='width: 100%%;'>%s</div>", toolMessage)
		c.WriteMessage(websocket.TextMessage, []byte(formattedContent))

		processed, err := wrapper.Tool.Process(ctx, prompt)
		if err != nil {
			log.Printf("error processing with tool %s: %v", wrapper.Name, err)
		}

		// Print the processed output for debugging
		log.Printf("Processed output from tool %s: %s", wrapper.Name, processed)

		if wrapper.Name == "teams" {
			teamsResponse = processed
		} else {
			allContent.WriteString(processed)
			allContent.WriteString("\n") // Separator between tool outputs
		}
	}

	retrievalInstructions := `\n\nYou have been provided with relevant document chunks retrieved from a retrieval-augmented generation (RAG) workflow. Use the information contained in these chunks to assist in generating your response only if it directly contributes to answering the user's prompt. You must ensure that:

	You do not explicitly reference or mention the existence of these chunks.
	You seamlessly incorporate relevant information into your response as if it were part of your own knowledge.
	If the provided chunks are not helpful for addressing the user's prompt, you may generate a response based on your general knowledge.`

	// Append the retrieval instructions to the final content
	allContent.WriteString(retrievalInstructions)

	// Append the Teams response to the final content
	allContent.WriteString(teamsResponse)

	promptDelimiter := "Now respond to the following question or instructions using the previous texts as reference. Ensure you always respond to the following: "
	prompt = fmt.Sprintf("%s\n%s", promptDelimiter, prompt)

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
	//params := t.GetParams()
	//log.Printf("WebSearchTool: Parameters: %v", params)
	// Print the search engine and endpoint for debugging
	//log.Printf("Search Engine: %s, Endpoint: %s", t.SearchEngine, t.Endpoint)

	// Perform search using GetSearXNGResults
	urls := web.GetSearXNGResults("https://search.intelligence.dev", input)

	urls = urls[:3]

	if len(urls) == 0 {
		return "", errors.New("no URLs found after filtering")
	}

	// Print the URLs for debugging
	log.Printf("URLs: %v", urls)

	// Fetch contents concurrently
	type result struct {
		content string
		err     error
	}

	var aggregatedContent strings.Builder

	for _, u := range urls {
		log.Printf("Fetching URL: %s", u)

		content, err := web.WebGetHandler(u)
		if err != nil {
			log.Printf("Failed to fetch content from URL %s: %v", u, err)
		}

		aggregatedContent.WriteString(content)
	}

	return aggregatedContent.String(), nil
}

// Enabled returns the enabled status of the tool.
func (t *WebSearchTool) Enabled() bool {
	return t.enabled
}

// SetParams configures the tool with provided parameters.
func (t *WebSearchTool) SetParams(params map[string]interface{}, config *Config) error {
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

// GetParams returns the tool's parameters.
func (t *WebSearchTool) GetParams() map[string]interface{} {
	// Get the params from the database
	params, err := db.GetToolMetadataByName("websearch")
	if err != nil {
		return nil
	}

	// map param.Params to a map[string]interface{}
	return map[string]interface{}{
		"enabled":       params.Params[1],
		"search_engine": params.Params[2],
		"endpoint":      params.Params[3],
		"top_n":         params.Params[4],
		"concurrency":   params.Params[0],
	}
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
		content, err := web.WebGetHandler(u)
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
func (t *WebGetTool) SetParams(params map[string]interface{}, config *Config) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	// Additional parameters can be set here if needed
	return nil
}

// GetParams returns the tool's parameters.
func (t *WebGetTool) GetParams() map[string]interface{} {
	return map[string]interface{}{
		"enabled": t.enabled,
	}
}

type RetrievalTool struct {
	enabled bool
	topN    int
}

// SetParams configures the tool with provided parameters.
func (t *RetrievalTool) SetParams(params map[string]interface{}, config *Config) error {
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

// GetParams returns the tool's parameters.
func (t *RetrievalTool) GetParams() map[string]interface{} {
	return map[string]interface{}{
		"enabled": t.enabled,
		"top_n":   t.topN,
	}
}

// Process is the main method that processes the input using sqlite fts5
// func (t *RetrievalTool) Process(ctx context.Context, input string) (string, error) {
// 	// Try to retrieve similar documents based on the input embedding
// 	documents, err := db.RetrieveTopNDocuments(ctx, input, 20)
// 	if err != nil {
// 		return "", err
// 	}

// 	// Print the retrieved documents for debugging
// 	log.Printf("Retrieved Documents: %v", documents)

// 	// Combine the documents into a single string
// 	var result strings.Builder
// 	for _, doc := range documents {
// 		result.WriteString(doc)
// 		result.WriteString("\n") // Separator between documents
// 	}

// 	return result.String(), nil
// }

func (t *RetrievalTool) Process(ctx context.Context, input string) (string, error) {
	// Use the IndexManager to create a search request based on the input
	searchRequest := indexManager.CreateSearchRequest(input, 3)

	// Perform the search to retrieve the top N documents
	searchResults, err := indexManager.SearchChunks(searchRequest)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve documents: %w", err)
	}

	// Print the retrieved search results for debugging
	log.Printf("Retrieved Documents: %v", searchResults)

	// Combine the retrieved documents' content into a single string
	var result strings.Builder
	for _, hit := range searchResults.Hits {
		doc, err := indexManager.GetDocument(hit.ID)
		if err != nil {
			log.Printf("Error retrieving document %s: %v\n", hit.ID, err)
			continue
		}

		// Extract the content of the document (assuming full_content field is stored)
		var content string
		doc.VisitFields(func(field index.Field) {
			// Print the field name for debugging
			log.Printf("Field Name: %s", field.Name())

			if field.Name() == "chunk" {
				result.WriteString(string(field.Value()))
			} else if field.Name() == "full_content" {
				// Print only the first 1000 characters of the full content
				log.Printf("Full content: %s", string(field.Value()))
				result.WriteString(string(field.Value()))
			}
		})

		// Add the document content to the result builder
		if content != "" {
			result.WriteString(content)
			result.WriteString("\n") // Separator between documents
		}
	}

	return result.String(), nil
}

// Enabled returns the enabled status of the tool.
func (t *RetrievalTool) Enabled() bool {
	return t.enabled
}

// TeamsTool is a new tool for integrating with the Teams service.
type TeamsTool struct {
	enabled       bool
	service       *ExternalService
	serviceConfig ServiceConfig
	client        LLMClient
	mu            sync.Mutex // To ensure thread-safe operations
}

// Process sends the user prompt to the Teams service and appends the response.
func (t *TeamsTool) Process(ctx context.Context, input string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// if !t.enabled {
	// 	return "", errors.New("TeamsTool is disabled")
	// }

	// Print the input for debugging
	log.Printf("TeamsTool: Input: %s", input)

	// Retrieve the text between {} as user prompt
	userPrompt := input[strings.Index(input, "{")+1 : strings.LastIndex(input, "}")]

	//ins := fmt.Sprintf("Prompt: %s - How can the previous prompt be enhanced with better instructions? Respond with the enhanced prompt only. Do not attempt to answer the prompt. Never output code.", userPrompt)
	// ins := fmt.Sprintf("Prompt: %s - Given the previous text, output a list of questions we should answer in order to respond accurately. Never output questions that do not serve to respod to the prompt. Stick to the topic and the topic only. Do not provide an answer to the Prompt. Only the set of questions.", userPrompt)
	ins := fmt.Sprintf("Prompt: %s - Rewrite the previous information as a list of three search engine queries. Return the list of queries only.", userPrompt)
	cpt := GetSystemTemplate("", ins)

	// Create a new LLM Client
	llmClient := NewLocalLLMClient("http://0.0.0.0:32185/v1", "", "")

	// Create the completion request payload
	payload := &CompletionRequest{
		Model:       "teams-model", // Update with the actual model name if needed
		Messages:    cpt.FormatMessages(nil),
		Temperature: 0.1, // Adjust parameters as needed
		TopP:        0.9,
		MaxTokens:   4096,
		Stream:      false, // As per requirement
	}

	// Print the payload for debugging
	log.Printf("TeamsTool: Payload: %v", payload)

	// Send the completion request to the Teams service
	resp, err := llmClient.SendCompletionRequest(payload)
	if err != nil {
		log.Printf("TeamsTool: Error sending completion request: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	// Parse the response
	var completionResp CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		log.Printf("TeamsTool: Error decoding completion response: %v", err)
		return "", err
	}

	if len(completionResp.Choices) == 0 {
		return "", errors.New("TeamsTool: No choices returned from completion response")
	}

	// Extract the content from the first choice
	responseContent := completionResp.Choices[0].Message.Content

	responseIns := "Your response must address the previous questions."

	// Append the response instructions to the response content
	responseContent = fmt.Sprintf("%s\n\n%s", responseContent, responseIns)

	// Print the response content for debugging
	log.Printf("TeamsTool: Response Content: %s", responseContent)

	// Append the response as a document
	err = SaveChatTurn(input, responseContent, time.Now().Format(time.RFC3339))
	if err != nil {
		log.Printf("TeamsTool: Failed to save chat turn: %v", err)
	}

	return responseContent, nil
}

// Enabled returns the enabled status of the tool.
func (t *TeamsTool) Enabled() bool {
	return t.enabled
}

// SetParams configures the tool with provided parameters, including starting the service if enabled.
func (t *TeamsTool) SetParams(params map[string]interface{}, config *Config) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}

	t.serviceConfig = config.Services[5]

	// args := []string{
	// 	"--model",
	// 	"/Users/arturoaquino/.eternal-v1/models-gguf/llama-3.2-3b/Llama-3.2-1B-Instruct-Q8_0.gguf",
	// 	"--port",
	// 	"32185",
	// 	"--host",
	// 	"0.0.0.0",
	// 	"--gpu-layers",
	// 	"99",
	// }

	// // Create a new ServiceConfig for the Teams tool
	// t.serviceConfig = ServiceConfig{
	// 	Name:    "Teams",
	// 	Command: "/Users/arturoaquino/Documents/code/llama.cpp/llama-server",
	// 	Args:    args,
	// }

	if t.enabled {
		// Initialize and start the ExternalService
		t.service = NewExternalService(t.serviceConfig, false) // Set verbose as needed
		if err := t.service.Start(context.Background()); err != nil {
			return fmt.Errorf("TeamsTool: failed to start external service: %w", err)
		}

		// Initialize the LLMClient pointing to the Teams service
		baseURL := fmt.Sprintf("http://%s:%d/v1", t.serviceConfig.Host, t.serviceConfig.Port)
		t.client = NewLocalLLMClient(baseURL, "", "") // Adjust APIKey if needed
	}

	return nil
}

// GetParams returns the tool's parameters.
func (t *TeamsTool) GetParams() map[string]interface{} {
	params := map[string]interface{}{
		"enabled": t.enabled,
	}

	// Include service configuration
	if t.serviceConfig.Name != "" {
		params["service_config"] = map[string]interface{}{
			"name":    t.serviceConfig.Name,
			"host":    t.serviceConfig.Host,
			"port":    t.serviceConfig.Port,
			"command": t.serviceConfig.Command,
			"args":    t.serviceConfig.Args,
		}
	}

	return params
}

// Helper function to convert interface{} to []string
func interfaceToStringSlice(input interface{}) []string {
	if input == nil {
		return []string{}
	}
	interfaceSlice, ok := input.([]interface{})
	if !ok {
		return []string{}
	}
	strSlice := make([]string, len(interfaceSlice))
	for i, v := range interfaceSlice {
		strSlice[i], _ = v.(string)
	}
	return strSlice
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
	case "teams":
		return &TeamsTool{}, nil
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

	// **New: Index in Bleve**
	docID := fmt.Sprintf("%d-%s", chat.ID, timestamp)
	// doc := map[string]interface{}{
	// 	"prompt":    prompt,
	// 	"response":  response,
	// 	"modelName": "assistant",
	// 	"timestamp": timestamp,
	// }

	fullDoc := fmt.Sprintf("%s\n%s", prompt, response)

	if err := indexManager.IndexDocumentChunk(docID, fullDoc, "assistant"); err != nil {
		return fmt.Errorf("failed to index document chunk: %w", err)
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
func handleToolToggle(c echo.Context, config *Config) error {
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
	UpdateWorkflowManagerForToolToggle(toolName, requestPayload.Enabled, config)

	// Respond with a success message
	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Tool '%s' has been %s", toolName, map[bool]string{true: "enabled", false: "disabled"}[requestPayload.Enabled]),
	})
}

// HandleGetTools returns the list of tools and their enabled status.
func handleGetTools(c echo.Context) error {
	tools, err := db.GetToolsMetadata()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch tools metadata",
		})
	}

	return c.JSON(http.StatusOK, tools)
}

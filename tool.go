// tool.go

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"manifold/internal/web"

	"github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
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
	// Get the list of enabled tools and print their names
	log.Printf("Enabled tools: %v", wm.ListTools())

	var result string
	currentInput := prompt
	for _, wrapper := range wm.tools {
		processed, err := wrapper.Tool.Process(ctx, currentInput)
		if err != nil {
			return "", fmt.Errorf("error processing with tool %s: %w", wrapper.Name, err)
		}
		result += processed
		currentInput = processed
	}
	if result == "" {
		return prompt, nil
	}
	return result, nil
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

	// Remove unwanted URLs (already done in GetSearXNGResults, but double-checking)
	urls = web.RemoveUnwantedURLs(urls)

	if len(urls) == 0 {
		return "", errors.New("no URLs found after filtering")
	}

	// Limit to TopN
	if t.TopN > len(urls) {
		t.TopN = len(urls)
	}
	topURLs := urls[:t.TopN]

	// Fetch contents concurrently
	type result struct {
		content string
		err     error
	}

	resultsChan := make(chan result, t.TopN)
	var wg sync.WaitGroup

	// Semaphore to limit concurrency
	semaphore := make(chan struct{}, t.Concurrency)

	for _, u := range topURLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			content, err := web.WebGetHandler(ctx, url)
			if err != nil {
				log.Printf("Failed to fetch content from URL %s: %v", url, err)
				resultsChan <- result{content: "", err: err}
				return
			}
			resultsChan <- result{content: content, err: nil}
		}(u)
	}

	// Wait for all fetches to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var aggregatedContent strings.Builder
	for res := range resultsChan {
		if res.err == nil && res.content != "" {
			aggregatedContent.WriteString(res.content)
			aggregatedContent.WriteString("\n") // Separator between contents
		}
	}

	finalResult := aggregatedContent.String()
	if finalResult == "" {
		return input, errors.New("no tools processed the input or no result generated")
	}

	return finalResult, nil
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
	enabled  bool
	endpoint string
	topN     int
}

func (t *RetrievalTool) Enabled() bool {
	return t.enabled
}

func (t *RetrievalTool) SetParams(params map[string]interface{}) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	if endpoint, ok := params["endpoint"].(string); ok {
		t.endpoint = endpoint
	}
	if topN, ok := params["top_n"].(int); ok {
		t.topN = topN
	}
	return nil
}

func (t *RetrievalTool) Process(ctx context.Context, input string) (string, error) {
	req := new(RagRequest)
	req.Text = input
	req.TopN = 1

	// Create a Bleve match query for the input text
	searchRequest := bleve.NewSearchRequest(bleve.NewMatchQuery(req.Text))
	searchRequest.Size = req.TopN // Limit results to the topN documents

	// Print the request for debugging
	log.Printf("Search Request: %v", input)

	// Perform the search
	results, err := searchIndex.Search(searchRequest)
	if err != nil {
		return "", err
	}

	// Print the results for debugging
	log.Printf("Search results: %v", results)

	// Prepare a response structure to hold results
	type SearchResult struct {
		ID       string  `json:"id"`
		Score    float64 `json:"score"`
		Prompt   string  `json:"prompt"`
		Response string  `json:"response"`
	}
	var searchResults []SearchResult

	// Iterate over the search hits and retrieve the prompt and response fields
	for _, hit := range results.Hits {
		doc, err := searchIndex.Document(hit.ID)
		if err != nil {
			fmt.Printf("Error retrieving document %s: %v\n", hit.ID, err)
			continue
		}

		var prompt, response string

		// Visit each field in the document and capture the "prompt" and "response" fields
		doc.VisitFields(func(field index.Field) {
			fieldName := field.Name()
			fieldValue := string(field.Value())

			if fieldName == "prompt" {
				prompt = fieldValue
			}
			if fieldName == "response" {
				response = fieldValue
			}
		})

		// Append the search result to the response slice
		searchResults = append(searchResults, SearchResult{
			ID:       hit.ID,
			Score:    hit.Score,
			Prompt:   prompt,
			Response: response,
		})
	}

	// Return the prompt + response for each document as a concatenated string
	var response strings.Builder
	for _, sr := range searchResults {
		response.WriteString(sr.Prompt)
		response.WriteString("\n")
		response.WriteString(sr.Response)
		response.WriteString("\n\n")
	}

	// Print the search results for debugging
	log.Printf("Retrieval results: %s", searchResults)

	return response.String(), nil
}

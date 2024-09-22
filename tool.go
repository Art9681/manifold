// tool.go

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"manifold/internal/web"

	badger "github.com/dgraph-io/badger/v4"
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
		case "badgerkv":
			if enabled, ok := toolConfig.Parameters["enabled"].(bool); ok && enabled {
				tool := &BadgerTool{}
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

	for _, wrapper := range wm.tools {
		processed, err := wrapper.Tool.Process(ctx, prompt)
		if err != nil {
			log.Printf("error processing with tool %s: %v", wrapper.Name, err)
		}

		// Prepend the processed output to the prompt for the next tool without overwriting the original prompt
		prompt = processed + "\n" + prompt

	}

	return prompt, nil
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
			// Store the fetched content in the search index
			searchIndex.Index("search_result", res.content)

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

		// Store the fetched content in the search index
		searchIndex.Index(u, content)

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
	enabled bool
	db      *badger.DB
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

	// Initialize Badger database
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	badgerDbPath := filepath.Join(home, ".manifold/badger")
	opts := badger.DefaultOptions(badgerDbPath)
	t.db, err = badger.Open(opts)
	if err != nil {
		return err
	}

	return nil
}

// StoreDocument stores a document's content and its embedding in Badger.
func (t *RetrievalTool) StoreDocument(docID string, content string, embedding []float64) error {
	// Serialize the content and embedding
	data := struct {
		Content   string    `json:"content"`
		Embedding []float64 `json:"embedding"`
	}{
		Content:   content,
		Embedding: embedding,
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Store in Badger
	return t.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(docID), dataBytes)
	})
}

func (t *RetrievalTool) RetrieveDocuments(ctx context.Context, input string) (string, error) {
	// Generate the embedding for the input text
	inputEmbedding, err := GenerateEmbedding(input)
	if err != nil {
		return "", fmt.Errorf("error generating embedding for input text: %v", err)
	}

	// Iterate over all stored documents in Badger and calculate similarity
	var searchResults []struct {
		ID         string  `json:"id"`
		Content    string  `json:"content"`
		Similarity float64 `json:"similarity"`
	}

	err = t.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()

			var data struct {
				Content   string    `json:"content"`
				Embedding []float64 `json:"embedding"`
			}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &data)
			})
			if err != nil {
				return err
			}

			// Calculate similarity between the input embedding and the document embedding
			similarity := CosineSimilarity(inputEmbedding, data.Embedding)

			// Print the similarity and content for debugging
			log.Printf("Similarity with document ID %s: %f", key, similarity)

			if similarity > 0.5 { // Threshold can be adjusted
				searchResults = append(searchResults, struct {
					ID         string  `json:"id"`
					Content    string  `json:"content"`
					Similarity float64 `json:"similarity"`
				}{
					ID:         string(key),
					Content:    data.Content,
					Similarity: similarity,
				})
			}

			// Print the search results for debugging
			log.Printf("Search results: %v", searchResults)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	// Sort the search results by similarity in descending order
	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Similarity > searchResults[j].Similarity
	})

	// Limit the results to the top N documents
	if len(searchResults) > t.topN {
		searchResults = searchResults[:t.topN]
	}

	// Concatenate the content of the top N results
	var resultBuilder strings.Builder
	for _, result := range searchResults {
		resultBuilder.WriteString(fmt.Sprintf("Document ID: %s\n", result.ID))
		resultBuilder.WriteString(result.Content)
		resultBuilder.WriteString("\n\n")
	}

	return resultBuilder.String(), nil
}

// Process is the main method that processes the input using Badger.
func (t *RetrievalTool) Process(ctx context.Context, input string) (string, error) {
	// Try to retrieve similar documents based on the input embedding
	return t.RetrieveDocuments(ctx, input)
}

// Enabled returns the enabled status of the tool.
func (t *RetrievalTool) Enabled() bool {
	return t.enabled
}

type BadgerTool struct {
	db      *badger.DB
	enabled bool
}

func (t *BadgerTool) Process(ctx context.Context, input string) (string, error) {
	// BadgerTool may not need to process inputs directly.
	// You can either log an action or return a simple message indicating it's a store tool.
	log.Println("BadgerTool does not perform direct processing.")
	return input, nil
}

// StoreEmbedding stores the embedding in Badger with the document ID as the key.
func (t *BadgerTool) StoreEmbedding(docID string, embedding []float64, content string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	badgerDbPath := filepath.Join(home, ".manifold/badger")
	opts := badger.DefaultOptions(badgerDbPath)
	db, err := badger.Open(opts)
	if err != nil {
		return err
	}
	defer db.Close()

	// Serialize the content and embedding
	data := struct {
		Content   string    `json:"content"`
		Embedding []float64 `json:"embedding"`
	}{
		Content:   content,
		Embedding: embedding,
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshalling embedding data: %v", err)
	}

	// Log the data being stored for debugging
	log.Printf("Storing document ID %s with data: %s", docID, string(dataBytes))

	// Store the embedding using docID as the key
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(docID), dataBytes)
	})
}

// GetEmbedding retrieves the embedding from Badger using the document ID.
func (t *BadgerTool) GetEmbedding(docID string) (string, []float64, error) {
	var data struct {
		Content   string    `json:"content"`
		Embedding []float64 `json:"embedding"`
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}

	badgerDbPath := filepath.Join(home, ".manifold/badger")
	opts := badger.DefaultOptions(badgerDbPath)
	db, err := badger.Open(opts)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()

	// Retrieve the embedding from Badger
	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(docID))
		if err != nil {
			return err
		}

		// Print the data being retrieved for debugging
		log.Printf("Retrieving document ID %s", docID)

		// Print the item value for debugging
		log.Printf("Item value: %v", item)

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &data)
		})
	})

	if err != nil {
		return "", nil, fmt.Errorf("error unmarshalling embedding data: %v", err)
	}

	return data.Content, data.Embedding, nil
}

// SetParams configures the tool with provided parameters.
func (t *BadgerTool) SetParams(params map[string]interface{}) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	return nil
}

// Enabled returns the enabled status of the tool.
func (t *BadgerTool) Enabled() bool {
	return t.enabled
}

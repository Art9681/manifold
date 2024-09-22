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

	"manifold/internal/documents"
	"manifold/internal/web"

	"github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
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
	} else {
		t.topN = 3 // Default value
	}

	return nil
}

func (t *RetrievalTool) Process(ctx context.Context, input string) (string, error) {
	req := new(RagRequest)
	req.Text = input
	req.TopN = t.topN

	log.Printf("Checking if embedding for input text exists in Badger KV store")

	// First, try to retrieve the embedding from Badger
	badgerTool := &BadgerTool{}
	inputEmbedding, err := badgerTool.GetEmbedding("input_key")
	if err != nil {
		log.Printf("Embedding not found in Badger, generating a new one")
		// If not found, generate a new embedding
		inputEmbedding, err = GenerateEmbedding(input)
		if err != nil {
			return "", fmt.Errorf("error generating embedding for input text: %v", err)
		}
		// Store the new embedding in Badger
		err = badgerTool.StoreEmbedding("input_key", inputEmbedding)
		if err != nil {
			return "", fmt.Errorf("error storing embedding in Badger: %v", err)
		}
	}

	// Print the input embedding for debugging
	// log.Printf("Input embedding: %v", inputEmbedding)

	// Create a Bleve match query for the input text
	searchRequest := bleve.NewSearchRequest(bleve.NewMatchQuery(req.Text))
	//searchRequest.Size = req.TopN * 10 // Search with a larger size to filter later by similarity

	// Perform the search
	// results, err := searchIndex.Search(searchRequest)
	// if err != nil {
	// 	return "", err
	// }

	results, err := searchIndex.Search(searchRequest)
	if err != nil {
		return "", err
	}

	// Prepare a structure to hold results
	type SearchResult struct {
		ID         string  `json:"id"`
		Prompt     string  `json:"prompt"`
		Response   string  `json:"response"`
		Similarity float64 `json:"similarity"`
	}
	var searchResults []SearchResult

	// Iterate over the search hits and generate embeddings for each document in real-time
	for _, hit := range results.Hits {
		log.Printf("Retrieving document %s", hit.ID)
		doc, err := searchIndex.Document(hit.ID)
		if err != nil {
			log.Printf("Error retrieving document %s: %v", hit.ID, err)
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
				log.Printf("Response: %s", response)
			}
		})

		// Concatenate the prompt and response to generate the document content
		docContent := prompt + "\n" + response

		// Split the document content into chunks of 1024 tokens
		docChunks := documents.SplitTextByCount(docContent, 1024)

		for _, chunk := range docChunks {
			chunkEmbeddings, err := GenerateEmbedding(chunk)
			if err != nil {
				log.Printf("Error generating embedding for document chunk: %v", err)
				continue
			}

			// calculate the similarity between the input and the chunk
			similarity := CosineSimilarity(inputEmbedding, chunkEmbeddings)

			// Print the similarity for debugging
			log.Printf("Similarity between input and chunk: %f", similarity)

			// If the similarity is above a certain threshold, add the chunk to the search results
			if similarity > 0.7 {
				searchResults = append(searchResults, SearchResult{
					ID:         hit.ID,
					Prompt:     prompt,
					Response:   response,
					Similarity: similarity,
				})
			}
		}
	}

	// Sort the searchResults by similarity in descending order
	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Similarity > searchResults[j].Similarity
	})

	// Print the document id and similarity for debugging
	for _, sr := range searchResults {
		log.Printf("Document ID: %s, Similarity: %f", sr.ID, sr.Similarity)
	}

	// Limit the results to the top N documents
	if len(searchResults) > 3 {
		searchResults = searchResults[:3]
	}

	// Return the prompt + response for each document as a concatenated string
	var responseBuilder strings.Builder
	for _, sr := range searchResults {
		responseBuilder.WriteString(sr.Prompt)
		responseBuilder.WriteString("\n")
		responseBuilder.WriteString(sr.Response)
		responseBuilder.WriteString("\n\n")
	}

	// Print the search results for debugging
	log.Printf("Retrieval results: %v", searchResults)

	return responseBuilder.String(), nil
}

type BadgerTool struct {
	db      *badger.DB
	enabled bool
}

// Process manages the Badger store, storing embeddings and retrieving them during the process.
func (t *BadgerTool) Process(ctx context.Context, input string) (string, error) {
	// Open Badger database
	// Get user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	badgerDbPath := filepath.Join(home, ".manifold/badger")
	opts := badger.DefaultOptions(badgerDbPath)
	db, err := badger.Open(opts)
	if err != nil {
		return "", err
	}
	defer db.Close()

	// Save the input (for example purposes, we store the input as the key and its length as a value)
	err = db.Update(func(txn *badger.Txn) error {
		err := txn.Set([]byte("input_key"), []byte(input))
		return err
	})

	if err != nil {
		return "", err
	}

	// Read the value back
	var result string
	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("input_key"))
		if err != nil {
			return err
		}
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		result = string(val)
		return nil
	})

	if err != nil {
		return "", err
	}

	return result, nil
}

// StoreEmbedding stores the embedding in Badger with the document ID as the key.
func (t *BadgerTool) StoreEmbedding(docID string, embedding []float64) error {
	// Open Badger database
	// Get user home directory
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

	// Serialize the embedding
	embeddingBytes, err := json.Marshal(embedding)
	if err != nil {
		return err
	}

	// Store the embedding using docID as the key
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(docID), embeddingBytes)
	})
}

// GetEmbedding retrieves the embedding from Badger using the document ID.
func (t *BadgerTool) GetEmbedding(docID string) ([]float64, error) {
	var embedding []float64

	// Open Badger database
	// Get user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	badgerDbPath := filepath.Join(home, ".manifold/badger")
	opts := badger.DefaultOptions(badgerDbPath)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Retrieve the embedding from Badger
	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(docID))
		if err != nil {
			return err
		}
		embeddingBytes, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		err = json.Unmarshal(embeddingBytes, &embedding)
		return err
	})

	if err != nil {
		return nil, err
	}

	return embedding, nil
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

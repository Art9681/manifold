package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"manifold/internal/coderag"

	elasticsearch "github.com/elastic/go-elasticsearch/v8"
)

// Updated FunctionInfo matching main.go's struct
type FunctionInfo struct {
	Name          string    `json:"name"`
	Comments      string    `json:"comments"`
	Code          string    `json:"code"`
	CallsCount    int       `json:"calls_count"`
	CalledByCount int       `json:"called_by_count"`
	Embedding     []float64 `json:"embedding"`
}

// Updated RelationshipInfo matching main.go's struct
type RelationshipInfo struct {
	Name              string   `json:"name"`
	Comments          string   `json:"comments"`
	Code              string   `json:"code"`
	CallsCount        int      `json:"calls_count"`
	CalledByCount     int      `json:"called_by_count"`
	Calls             []string `json:"calls"`
	CalledBy          []string `json:"called_by"`
	Summary           string   `json:"summary"`
	CallsFilePaths    []string `json:"calls_file_paths"`
	CalledByFilePaths []string `json:"called_by_file_paths"`
}

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

type UsageMetrics struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

func main() {
	// Load configuration for OpenAI API
	cfg, err := coderag.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Initialize Elasticsearch client
	esClient, err := initElasticsearchClient()
	if err != nil {
		log.Fatalf("Error initializing Elasticsearch client: %v", err)
	}

	// Initialize the CodeIndex
	index := coderag.NewCodeIndex()

	// Index the repository and store data in Elasticsearch
	repoPath := "/path/to/repo"
	fmt.Printf("Indexing repository at: %s\n", repoPath)
	if err := indexRepositoryToElasticsearch(repoPath, cfg, esClient); err != nil {
		log.Fatalf("Indexing failed: %v", err)
	}
	fmt.Println("Indexing completed successfully.")

	// Start the API server in a separate goroutine
	go index.StartAPIServer(8080)

	// Create a buffered reader for user input
	reader := bufio.NewReader(os.Stdin)

	for {
		// Prompt the user for input
		fmt.Print("Enter your query (or type 'exit' to quit): ")
		prompt, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading input: %v", err)
			continue
		}

		// Trim any extra spaces/newlines from user input
		prompt = strings.TrimSpace(prompt)

		// Exit if the user types "exit"
		if strings.ToLower(prompt) == "exit" {
			fmt.Println("Exiting the application. Goodbye!")
			break
		}

		// Handle special commands like "refactor" to show refactoring opportunities
		if strings.ToLower(prompt) == "refactor" {
			displayRefactoringOpportunitiesFromElasticsearch(esClient)
			continue
		}

		// Handle the user's prompt by querying Elasticsearch
		relationshipInfo, err := queryElasticsearchForFunction(prompt, esClient)
		if err != nil {
			fmt.Printf("Error processing query: %v\n", err)
			continue
		}

		// Display the relationship information, comments, and source code
		displayRelationshipInfo(relationshipInfo)
	}
}

// initElasticsearchClient initializes the Elasticsearch client.
func initElasticsearchClient() (*elasticsearch.Client, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{
			"https://192.168.0.153:9200",
		},
		Username: "elastic",
		Password: "changeme",
		// Insecure transport is used for simplicity in this example
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	return elasticsearch.NewClient(cfg)
}

// generateEmbeddings makes a request to the local embeddings service to generate embeddings for a given input text.
func generateEmbeddings(text string) ([]float64, error) {
	url := "http://localhost:32184/embeddings"

	// Create the embeddings request payload
	payload := EmbeddingRequest{
		Input:          []string{text},
		Model:          "nomic-embed-text-v1.5",
		EncodingFormat: "json",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to embeddings service: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, fmt.Errorf("error response from embeddings service: %s", body)
	}

	// Parse the response from the embeddings service
	var embeddingResponse EmbeddingResponse
	if err := json.NewDecoder(res.Body).Decode(&embeddingResponse); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	// Extract the embeddings from the response
	if len(embeddingResponse.Data) == 0 {
		return nil, fmt.Errorf("no embeddings found in response")
	}

	vector := embeddingResponse.Data[0].Embedding

	return vector, nil
}

// indexRepositoryToElasticsearch indexes the repository data into Elasticsearch with embeddings.
func indexRepositoryToElasticsearch(repoPath string, cfg *coderag.Config, esClient *elasticsearch.Client) error {
	// Validate the Elasticsearch connection before proceeding
	res, err := esClient.Info()
	if err != nil {
		return fmt.Errorf("error connecting to Elasticsearch: %v", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("error response from Elasticsearch: %s", res.String())
	}

	// Index repository data using the existing logic in coderag
	index := coderag.NewCodeIndex()
	if err := index.IndexRepository(repoPath, cfg); err != nil {
		return err
	}

	// Convert and ingest each function data into Elasticsearch as it's processed
	var wg sync.WaitGroup
	for _, function := range index.Functions {
		wg.Add(1)
		go func(function coderag.FunctionInfo) {
			defer wg.Done()
			// Generate embeddings for the function's code
			embedding, err := generateEmbeddings(function.Code)
			if err != nil {
				log.Printf("error generating embeddings: %v", err)
				return
			}

			// Add embeddings to the function data
			functionWithEmbedding := FunctionInfo{
				Name:          function.Name,
				Comments:      function.Comments,
				Code:          function.Code,
				CallsCount:    len(function.Calls),
				CalledByCount: len(function.CalledBy),
				Embedding:     embedding,
			}

			docJSON, err := json.Marshal(functionWithEmbedding)
			if err != nil {
				log.Printf("error marshaling function data: %v", err)
				return
			}

			// Ingest each document into Elasticsearch immediately
			res, err := esClient.Index(
				"functions",
				bytes.NewReader(docJSON),
			)
			if err != nil {
				log.Printf("error indexing function data: %v", err)
				return
			}
			defer res.Body.Close()

			if res.IsError() {
				log.Printf("error response when indexing document: %s", res.String())
				return
			}
		}(*function)
	}
	wg.Wait()

	return nil
}

// queryElasticsearchForFunction queries Elasticsearch for function details based on the user's prompt.
func queryElasticsearchForFunction(prompt string, esClient *elasticsearch.Client) (*RelationshipInfo, error) {
	query := fmt.Sprintf(`{
		"query": {
			"match": {
				"name": "%s"
			}
		}
	}`, prompt)

	res, err := esClient.Search(
		esClient.Search.WithContext(context.Background()),
		esClient.Search.WithIndex("functions"),
		esClient.Search.WithBody(bytes.NewReader([]byte(query))),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying Elasticsearch: %v", err)
	}
	defer res.Body.Close()

	// Parse the search results
	var searchResults map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&searchResults); err != nil {
		return nil, fmt.Errorf("error parsing search results: %v", err)
	}

	// Extract the function data from the search results
	hits, ok := searchResults["hits"].(map[string]interface{})["hits"].([]interface{})
	if !ok || len(hits) == 0 {
		return nil, fmt.Errorf("no function found for query: %s", prompt)
	}

	source, ok := hits[0].(map[string]interface{})["_source"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data format in search results")
	}

	functionInfo := RelationshipInfo{
		Name:          source["name"].(string),
		Comments:      source["comments"].(string),
		Code:          source["code"].(string),
		CallsCount:    int(source["calls_count"].(float64)),
		CalledByCount: int(source["called_by_count"].(float64)),
		// Populate other fields as necessary, e.g., Calls, CalledBy, Summary, etc.
	}

	return &functionInfo, nil
}

// displayRefactoringOpportunitiesFromElasticsearch fetches and displays refactoring opportunities from Elasticsearch.
func displayRefactoringOpportunitiesFromElasticsearch(esClient *elasticsearch.Client) {
	query := `{
		"query": {
			"match_all": {}
		}
	}`

	res, err := esClient.Search(
		esClient.Search.WithContext(context.Background()),
		esClient.Search.WithIndex("refactoring_opportunities"),
		esClient.Search.WithBody(bytes.NewReader([]byte(query))),
	)
	if err != nil {
		log.Fatalf("Error querying refactoring opportunities: %v", err)
	}
	defer res.Body.Close()

	// Parse the search results
	var searchResults map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&searchResults); err != nil {
		log.Fatalf("Error parsing search results: %v", err)
	}

	// Display the refactoring opportunities
	hits, ok := searchResults["hits"].(map[string]interface{})["hits"].([]interface{})
	if !ok || len(hits) == 0 {
		fmt.Println("No refactoring opportunities found.")
		return
	}

	fmt.Println("\nRefactoring Opportunities:")
	for i, hit := range hits {
		source, ok := hit.(map[string]interface{})["_source"].(map[string]interface{})
		if !ok {
			continue
		}
		fmt.Printf("%d. %s\n   Location: %s\n   Severity: %s\n\n", i+1, source["description"], source["location"], source["severity"])
	}
}

// displayRelationshipInfo displays detailed information about a function or method.
func displayRelationshipInfo(info *RelationshipInfo) {
	fmt.Printf("\nFunction: %s\n", info.Name)
	fmt.Printf("Total Calls: %d\n", info.CallsCount)
	fmt.Printf("Total Called By: %d\n", info.CalledByCount)

	// Display function comments if available
	if info.Comments != "" {
		fmt.Printf("\nComments:\n%s\n", info.Comments)
	} else {
		fmt.Println("\nComments: None")
	}

	// Display the summary if available
	if info.Summary != "" {
		fmt.Printf("\nSummary:\n%s\n", info.Summary)
	} else {
		fmt.Println("\nSummary: None")
	}

	// Display the source code of the function
	fmt.Printf("\nSource Code:\n%s\n", info.Code)

	// Display the functions this function calls
	fmt.Printf("\nFunctions Called by %s:\n", info.Name)
	if len(info.Calls) > 0 {
		for i, calledFunc := range info.Calls {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, calledFunc, info.CallsFilePaths[i])
		}
	} else {
		fmt.Println("  None")
	}

	// Display the functions that call this function
	fmt.Printf("\nFunctions Calling %s:\n", info.Name)
	if len(info.CalledBy) > 0 {
		for i, callerFunc := range info.CalledBy {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, callerFunc, info.CalledByFilePaths[i])
		}
	} else {
		fmt.Println("  None")
	}
	fmt.Println()
}

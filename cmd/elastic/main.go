package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"manifold/internal/coderag"

	elasticsearch "github.com/elastic/go-elasticsearch/v8"
)

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
	repoPath := "/Users/arturoaquino/Documents/manifold"
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
	//cert, _ := os.ReadFile("ca.crt")

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

// indexRepositoryToElasticsearch indexes the repository data into Elasticsearch.
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
	for _, function := range index.Functions {
		docJSON, err := json.Marshal(function)
		if err != nil {
			return fmt.Errorf("error marshaling function data: %v", err)
		}

		// Ingest each document into Elasticsearch immediately
		res, err := esClient.Index(
			"functions",
			bytes.NewReader(docJSON),
		)
		if err != nil {
			return fmt.Errorf("error indexing function data: %v", err)
		}
		defer res.Body.Close()

		if res.IsError() {
			return fmt.Errorf("error response when indexing document: %s", res.String())
		}
	}

	return nil
}

// queryElasticsearchForFunction queries Elasticsearch for function details based on the user's prompt.
func queryElasticsearchForFunction(prompt string, esClient *elasticsearch.Client) (*coderag.RelationshipInfo, error) {
	query := fmt.Sprintf(`{
		"query": {
			"match": {
				"function_name": "%s"
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
	hits := searchResults["hits"].(map[string]interface{})["hits"].([]interface{})
	if len(hits) == 0 {
		return nil, fmt.Errorf("no function found for query: %s", prompt)
	}

	source := hits[0].(map[string]interface{})["_source"].(map[string]interface{})
	functionInfo := coderag.RelationshipInfo{
		FunctionName:  source["function_name"].(string),
		Comments:      source["comments"].(string),
		Code:          source["code"].(string),
		TotalCalls:    int(source["total_calls"].(float64)),
		TotalCalledBy: int(source["total_called_by"].(float64)),
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
	hits := searchResults["hits"].(map[string]interface{})["hits"].([]interface{})
	if len(hits) == 0 {
		fmt.Println("No refactoring opportunities found.")
		return
	}

	fmt.Println("\nRefactoring Opportunities:")
	for i, hit := range hits {
		source := hit.(map[string]interface{})["_source"].(map[string]interface{})
		fmt.Printf("%d. %s\n   Location: %s\n   Severity: %s\n\n", i+1, source["description"], source["location"], source["severity"])
	}
}

// displayRelationshipInfo displays detailed information about a function or method.
func displayRelationshipInfo(info *coderag.RelationshipInfo) {
	fmt.Printf("\nFunction: %s\n", info.FunctionName)
	fmt.Printf("Total Calls: %d\n", info.TotalCalls)
	fmt.Printf("Total Called By: %d\n", info.TotalCalledBy)

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
	fmt.Printf("\nFunctions Called by %s:\n", info.FunctionName)
	if len(info.Calls) > 0 {
		for i, calledFunc := range info.Calls {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, calledFunc, info.CallsFilePaths[i])
		}
	} else {
		fmt.Println("  None")
	}

	// Display the functions that call this function
	fmt.Printf("\nFunctions Calling %s:\n", info.FunctionName)
	if len(info.CalledBy) > 0 {
		for i, callerFunc := range info.CalledBy {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, callerFunc, info.CalledByFilePaths[i])
		}
	} else {
		fmt.Println("  None")
	}
	fmt.Println()
}

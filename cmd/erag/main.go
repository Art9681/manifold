package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	bleve "github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
)

func main() {
	// Define the index path (you need to have an existing index)
	indexPath := "/Users/arturoaquino/.manifold/search.bleve"
	queryText := "WHat is the DNS issue with MacOS Sequoia?" // Replace this with the input text you want to search for
	topN := 10                                               // Number of top documents to return

	// Check if the index exists
	exists, err := IndexExists(indexPath)
	if err != nil {
		log.Fatalf("Error checking if index exists: %v", err)
	}

	if !exists {
		log.Fatalf("Index at path %s does not exist", indexPath)
	}

	// Open the existing index
	idx, err := OpenIndex(indexPath)
	if err != nil {
		log.Fatalf("Error opening index: %v", err)
	}
	defer idx.Close()

	// Perform the search
	searchResults, err := idx.SearchTopN(queryText, topN)
	if err != nil {
		log.Fatalf("Error performing search: %v", err)
	}

	fmt.Println(searchResults.Hits)

	// Print the search results
	fmt.Printf("Search Results for '%s':\n", queryText)
	for i, hit := range searchResults.Hits {
		fmt.Printf("%d. ID: %s, Score: %f\n", i+1, hit.ID, hit.Score)

		// Retrieve and print document contents
		doc, err := idx.GetDocumentByID(hit.ID)
		if err != nil {
			log.Printf("Error retrieving document %s: %v", hit.ID, err)
			continue
		}

		docJson, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			log.Printf("Error converting document %s to JSON: %v", hit.ID, err)
			continue
		}

		fmt.Printf("Document content:\n%s\n", string(docJson))
	}
}

// SearchTopN performs a search and returns the top N results.
func (idx *Index) SearchTopN(query string, topN int) (*bleve.SearchResult, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return nil, fmt.Errorf("index is closed")
	}

	searchQuery := bleve.NewMatchQuery(query)
	searchRequest := bleve.NewSearchRequest(searchQuery)
	searchRequest.Size = topN // Limit the number of results to topN

	return idx.bleveIndex.Search(searchRequest)
}

// GetDocumentByID retrieves the stored fields of a document by its ID.
func (idx *Index) GetDocumentByID(id string) (string, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return "", fmt.Errorf("index is closed")
	}

	doc, err := idx.bleveIndex.Document(id)
	if err != nil {
		return "", err
	}

	// Visit each field in the document and convert the value to a string
	doc.VisitFields(func(field index.Field) {
		fieldName := field.Name()
		fieldValue := string(field.Value()) // Convert byte slice to string

		fmt.Printf("Field: %s, Value: %s\n", fieldName, fieldValue)
	})

	return "", nil
}

// IndexExists checks if an index exists at the given path.
func IndexExists(indexPath string) (bool, error) {
	_, err := os.Stat(indexPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// OpenIndex opens an existing Bleve index at the given path.
func OpenIndex(indexPath string) (*Index, error) {
	bleveIndex, err := bleve.Open(indexPath)
	if err != nil {
		return nil, err
	}
	return &Index{bleveIndex: bleveIndex, path: indexPath}, nil
}

// Index represents a Bleve index with common operations.
type Index struct {
	bleveIndex bleve.Index
	path       string
	mu         sync.Mutex
	closed     bool
}

// Close closes the index.
func (idx *Index) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.closed {
		return nil
	}
	idx.closed = true
	return idx.bleveIndex.Close()
}

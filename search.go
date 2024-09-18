package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
)

// GenerateMD5Hash generates an MD5 hash for a given chunk of text
func GenerateMD5Hash(text string) string {
	hash := md5.New()                    // Create a new MD5 hash instance
	hash.Write([]byte(text))             // Write the text as bytes into the hash
	hashBytes := hash.Sum(nil)           // Compute the final hash
	return hex.EncodeToString(hashBytes) // Convert the hash to a hexadecimal string
}

// Index represents a Bleve index with common operations.
type Index struct {
	bleveIndex bleve.Index
	path       string
	mu         sync.Mutex
	closed     bool
}

// NewIndex creates a new Bleve index at the given path.
func NewIndex(indexPath string) (*Index, error) {
	mapping := bleve.NewIndexMapping()
	bleveIndex, err := bleve.New(indexPath, mapping)
	if err != nil {
		log.Printf("Error creating new index: %v", err)
		return nil, err
	}
	return &Index{bleveIndex: bleveIndex, path: indexPath}, nil
}

// OpenIndex opens an existing Bleve index at the given path.
func OpenIndex(indexPath string) (*Index, error) {
	bleveIndex, err := bleve.Open(indexPath)
	if err != nil {
		return nil, err
	}
	return &Index{bleveIndex: bleveIndex, path: indexPath}, nil
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

// Delete removes a document from the index.
func (idx *Index) Delete(id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.closed {
		return errors.New("index is closed")
	}
	return idx.bleveIndex.Delete(id)
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

// DeleteIndex closes the index (if not already closed) and deletes the index directory.
func (idx *Index) DeleteIndex() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if !idx.closed {
		idx.closed = true
		if err := idx.bleveIndex.Close(); err != nil {
			return err
		}
	}
	return os.RemoveAll(idx.path)
}

// Index adds or updates a document in the index.
func (idx *Index) Index(id string, data interface{}) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.closed {
		return errors.New("index is closed")
	}
	return idx.bleveIndex.Index(id, data)
}

// BatchIndex allows indexing of multiple documents in one batch.
func (idx *Index) BatchIndex(documents map[string]interface{}) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return errors.New("index is closed")
	}

	batch := idx.bleveIndex.NewBatch()
	for id, doc := range documents {
		err := batch.Index(id, doc)
		if err != nil {
			return err
		}
	}

	return idx.bleveIndex.Batch(batch)
}

// Search performs a search query on the index.
func (idx *Index) Search(query string) (*bleve.SearchResult, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.closed {
		return nil, errors.New("index is closed")
	}
	searchQuery := bleve.NewMatchQuery(query)
	searchRequest := bleve.NewSearchRequest(searchQuery)
	return idx.bleveIndex.Search(searchRequest)
}

// FacetedSearch performs a faceted search with the given field for aggregations.
func (idx *Index) FacetedSearch(query string, facetField string) (*bleve.SearchResult, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return nil, errors.New("index is closed")
	}

	searchQuery := bleve.NewMatchQuery(query)
	searchRequest := bleve.NewSearchRequest(searchQuery)

	// Add a facet for the provided field
	searchRequest.AddFacet(facetField, bleve.NewFacetRequest(facetField, 10))

	return idx.bleveIndex.Search(searchRequest)
}

// BoostedSearch performs a search and boosts documents that match a particular field or criteria.
func (idx *Index) BoostedSearch(query string, boostField string, boostValue float64) (*bleve.SearchResult, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return nil, errors.New("index is closed")
	}

	// Create a boosted query
	mainQuery := bleve.NewMatchQuery(query)
	boostedQuery := bleve.NewQueryStringQuery(boostField)
	boostedQuery.SetBoost(boostValue)

	// Combine both queries
	booleanQuery := bleve.NewBooleanQuery()
	booleanQuery.AddMust(mainQuery)
	booleanQuery.AddShould(boostedQuery)

	searchRequest := bleve.NewSearchRequest(booleanQuery)
	return idx.bleveIndex.Search(searchRequest)
}

// DynamicQuery allows constructing queries with different conditions.
func (idx *Index) DynamicQuery(match string, rangeField string, start, end interface{}) (*bleve.SearchResult, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return nil, errors.New("index is closed")
	}

	// Match query
	matchQuery := bleve.NewMatchQuery(match)

	// Range query
	startFloat, _ := start.(float64)
	endFloat, _ := end.(float64)
	rangeQuery := bleve.NewNumericRangeQuery(&startFloat, &endFloat)
	rangeQuery.SetField(rangeField)

	// Combine both queries
	booleanQuery := bleve.NewBooleanQuery()
	booleanQuery.AddMust(matchQuery)
	booleanQuery.AddMust(rangeQuery)

	searchRequest := bleve.NewSearchRequest(booleanQuery)
	return idx.bleveIndex.Search(searchRequest)
}

func ExpandQueryWithSynonyms(queryStr string, synonyms map[string][]string) query.Query {
	matchQuery := bleve.NewMatchQuery(queryStr)
	booleanQuery := bleve.NewBooleanQuery()

	// Add the original query
	booleanQuery.AddMust(matchQuery)

	// Add queries for synonyms
	for word, synonymList := range synonyms {
		if word == queryStr {
			for _, synonym := range synonymList {
				synonymQuery := bleve.NewMatchQuery(synonym)
				booleanQuery.AddShould(synonymQuery)
			}
		}
	}

	return booleanQuery
}

type Cache struct {
	results map[string]*bleve.SearchResult
	mu      sync.Mutex
}

// GetCachedResult returns a cached result if available.
func (cache *Cache) GetCachedResult(query string) (*bleve.SearchResult, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	result, found := cache.results[query]
	return result, found
}

// StoreResult caches a search result for a specific query.
func (cache *Cache) StoreResult(query string, result *bleve.SearchResult) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.results[query] = result
}

// Create a new document mapping with stored fields
func createMapping() *mapping.IndexMappingImpl {
	docMapping := bleve.NewDocumentMapping()

	// Define a field mapping for the prompt and response
	textFieldMapping := bleve.NewTextFieldMapping()
	textFieldMapping.Store = true // Ensure the field is stored for retrieval

	// Define a field mapping for chunk content
	chunkFieldMapping := bleve.NewTextFieldMapping()
	chunkFieldMapping.Store = true

	// Add field mappings to the document mapping
	docMapping.AddFieldMappingsAt("prompt", textFieldMapping)
	docMapping.AddFieldMappingsAt("response", textFieldMapping)
	//docMapping.AddFieldMappingsAt("chunk", chunkFieldMapping)

	// Create an index mapping and add the document mapping
	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("chatdoc", docMapping)

	return indexMapping
}
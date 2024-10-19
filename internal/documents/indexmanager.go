package documents

import (
	"fmt"
	"log"

	"github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
)

// IndexManager handles the indexing and retrieval of document chunks.
type IndexManager struct {
	Index bleve.Index
}

// NewIndexManager creates a new instance of IndexManager.
func NewIndexManager(indexPath string) (*IndexManager, error) {
	// Open or create a Bleve index
	index, err := bleve.Open(indexPath)
	if err != nil {
		// If the index does not exist, create a new one
		mapping := bleve.NewIndexMapping()
		index, err = bleve.New(indexPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("failed to create bleve index: %w", err)
		}
	}

	return &IndexManager{Index: index}, nil
}

// IndexFullDocument stores the entire document in the Bleve index.
func (im *IndexManager) IndexFullDocument(docID, content, filePath string) error {
	doc := map[string]interface{}{
		"full_content": content,
		"file_path":    filePath,
	}

	if err := im.Index.Index(docID, doc); err != nil {
		log.Printf("Error indexing full document: %v", err)
		return err
	}
	return nil
}

// IndexDocumentChunk stores a document chunk in the Bleve index.
func (im *IndexManager) IndexDocumentChunk(docID, chunk, filePath string) error {
	doc := map[string]interface{}{
		"chunk":     chunk,
		"file_path": filePath,
	}

	if err := im.Index.Index(docID, doc); err != nil {
		log.Printf("Error indexing document chunk: %v", err)
		return err
	}
	return nil
}

// CreateSearchRequest creates a search request based on the input text and desired top N results.
func (im *IndexManager) CreateSearchRequest(queryText string, topN int) *bleve.SearchRequest {
	query := bleve.NewMatchQuery(queryText)
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = topN
	return searchRequest
}

// SearchChunks performs a search on the index using a given search request.
func (im *IndexManager) SearchChunks(searchRequest *bleve.SearchRequest) (*bleve.SearchResult, error) {
	return im.Index.Search(searchRequest)
}

// GetDocument retrieves a document by its ID from the index.
func (im *IndexManager) GetDocument(docID string) (index.Document, error) {
	return im.Index.Document(docID)
}

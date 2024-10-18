package documents

import (
	"fmt"
	"log"

	"github.com/blevesearch/bleve"
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

// SearchChunks retrieves chunks based on the query string.
func (im *IndexManager) SearchChunks(queryString string) ([]string, error) {
	query := bleve.NewQueryStringQuery(queryString)
	search := bleve.NewSearchRequest(query)
	searchResults, err := im.Index.Search(search)
	if err != nil {
		return nil, fmt.Errorf("failed to search chunks: %w", err)
	}

	var results []string
	for _, hit := range searchResults.Hits {
		doc, err := im.Index.Document(hit.ID)
		if err != nil {
			log.Printf("failed to load document for ID %s: %v", hit.ID, err)
			continue
		}

		// Extract the "chunk" or "full_content" field from the document
		for _, field := range doc.Fields {
			if field.Name() == "chunk" || field.Name() == "full_content" {
				results = append(results, string(field.Value()))
			}
		}
	}
	return results, nil
}

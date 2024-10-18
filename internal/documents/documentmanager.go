package documents

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
)

type Document struct {
	PageContent string
	Metadata    map[string]string
}

type DocumentManager struct {
	Documents    []Document
	ChunkSize    int
	OverlapSize  int
	IndexManager *IndexManager
}

// NewDocumentManager initializes a DocumentManager with chunk, overlap sizes, and an optional IndexManager.
func NewDocumentManager(chunkSize, overlapSize int, indexManager *IndexManager) *DocumentManager {
	return &DocumentManager{
		ChunkSize:    chunkSize,
		OverlapSize:  overlapSize,
		IndexManager: indexManager,
	}
}

// SplitDocuments splits the content of documents based on their language-specific separators and indexes them.
func (dm *DocumentManager) SplitDocuments() (map[string][]string, error) {
	splits := make(map[string][]string)

	for _, doc := range dm.Documents {
		// Get language from metadata
		language, err := getLanguageFromMetadata(doc.Metadata)
		if err != nil {
			// If language is not found, default to splitting by lines
			language = DEFAULT
		}

		splitter, err := getSplitterForLanguage(language)
		if err != nil {
			return nil, err
		}

		splitter.ChunkSize = dm.ChunkSize
		splitter.OverlapSize = dm.OverlapSize
		splitter.LengthFunction = func(s string) int { return len(s) }

		// Split the content
		chunks := splitter.SplitText(doc.PageContent)

		// Generate a unique key for the document
		key := generateDocumentKey(doc)
		splits[key] = chunks

		// Index the chunks if IndexManager is set
		if dm.IndexManager != nil {
			for idx, chunk := range chunks {
				docID := fmt.Sprintf("%s-%d", key, idx)
				err := dm.IndexManager.IndexDocumentChunk(docID, chunk, doc.Metadata["source"])
				if err != nil {
					return nil, fmt.Errorf("failed to index chunk: %w", err)
				}
			}
		}
	}

	return splits, nil
}

// IngestDocument ingests a single document into the DocumentManager and indexes it.
func (dm *DocumentManager) IngestDocument(doc Document) {
	dm.Documents = append(dm.Documents, doc)

	// Index the full document content if IndexManager is set
	if dm.IndexManager != nil {
		docID := generateDocumentKey(doc)
		err := dm.IndexManager.IndexFullDocument(docID, doc.PageContent, doc.Metadata["source"])
		if err != nil {
			fmt.Printf("Failed to index full document: %s\n", err)
		}
	}
}

// IngestDocuments ingests multiple documents into the DocumentManager.
func (dm *DocumentManager) IngestDocuments(docs []Document) {
	dm.Documents = append(dm.Documents, docs...)
}

// Helper function to generate a unique key for the document
func generateDocumentKey(doc Document) string {
	if source, ok := doc.Metadata["source"]; ok {
		return source
	}

	// Fallback: use a hash of the content as the key
	hasher := sha1.New()
	hasher.Write([]byte(doc.PageContent))
	return hex.EncodeToString(hasher.Sum(nil))
}

// Helper function to get the language from document metadata.
func getLanguageFromMetadata(metadata map[string]string) (Language, error) {
	if langStr, ok := metadata["language"]; ok {
		return Language(langStr), nil
	}

	if fileType, ok := metadata["file_type"]; ok {
		// Map file extensions to Language constants
		switch fileType {
		case ".py":
			return PYTHON, nil
		case ".go":
			return GO, nil
		case ".html", ".htm":
			return HTML, nil
		case ".js":
			return JS, nil
		case ".ts":
			return TS, nil
		case ".md":
			return MARKDOWN, nil
		case ".json":
			return JSON, nil
		default:
			return DEFAULT, fmt.Errorf("unsupported file type: %s", fileType)
		}
	}

	return DEFAULT, errors.New("language or file_type not found in metadata")
}

// Helper function to get the splitter for a specific language.
func getSplitterForLanguage(language Language) (*RecursiveCharacterTextSplitter, error) {
	if language != DEFAULT {
		return FromLanguage(language)
	}

	// Default splitter: split by lines and paragraphs
	return &RecursiveCharacterTextSplitter{
		Separators:       []string{"\n\n", "\n", " ", ""},
		ChunkSize:        1000, // Default chunk size
		OverlapSize:      200,  // Default overlap size
		IsSeparatorRegex: false,
	}, nil
}

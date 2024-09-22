package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"manifold/internal/documents"
)

type StoreDocumentRequest struct {
	Text     string            `json:"text"`
	Metadata map[string]string `json:"metadata"`
}

// Function to send a document to the /v1/store-document endpoint
func sendDocumentToEndpoint(doc documents.Document) error {
	url := "http://localhost:32180/v1/store-document"
	reqBody := StoreDocumentRequest{
		Text: doc.PageContent,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	log.Println("Document stored successfully")
	return nil
}

func main() {
	// Setup parameters for GitLoader
	repoPath := "/Users/arturoaquino/Documents/manifold" // Path where the repo should be cloned
	cloneURL := "git@github.com:your/repo.git"           // Git URL of the repository
	branch := "main"                                     // Branch to be checked out
	privateKeyPath := "/path/to/private/key"             // SSH Private key path
	fileFilter := func(path string) bool {               // Custom file filter (optional)
		// Example: only load Markdown files
		return filepath.Ext(path) == ".go"
	}
	insecureSkipVerify := true // Set to true if skipping host key verification (for testing)

	// Create a new GitLoader instance
	gitLoader := documents.NewGitLoader(repoPath, cloneURL, branch, privateKeyPath, fileFilter, insecureSkipVerify)

	// Load documents from the repository
	docs, err := gitLoader.Load()
	if err != nil {
		log.Fatalf("Error loading documents: %v", err)
		os.Exit(1)
	}

	// Loop over each document and send it to the /v1/store-document endpoint
	for _, doc := range docs {
		err := sendDocumentToEndpoint(doc)
		if err != nil {
			log.Printf("Error sending document: %v", err)
		}
	}
}

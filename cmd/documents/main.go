package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/documents"
)

var docManager *documents.DocumentManager
var indexManager *documents.IndexManager

func main() {
	// Initialize IndexManager
	var err error
	indexManager, err = documents.NewIndexManager("/tmp/bleve_index")
	if err != nil {
		fmt.Println("Failed to initialize index:", err)
		return
	}

	// Initialize the DocumentManager with chunk size, overlap size, and IndexManager
	docManager = documents.NewDocumentManager(2048, 0, indexManager)

	// Setup HTTP routes
	http.HandleFunc("/ingest/git", handleGitIngest)
	http.HandleFunc("/ingest/pdf", handlePDFIngest)
	http.HandleFunc("/split", handleSplitDocuments)
	http.HandleFunc("/query", handleQueryChunks)

	// Start HTTP server
	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Failed to start server:", err)
	}
}

// handleQueryChunks handles querying indexed document chunks by file path or content.
func handleQueryChunks(w http.ResponseWriter, r *http.Request) {
	queryString := r.URL.Query().Get("query")
	if queryString == "" {
		http.Error(w, "Missing query parameter", http.StatusBadRequest)
		return
	}

	// Search for chunks related to the query
	chunks, err := indexManager.SearchChunks(queryString)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to search indexed chunks: %s", err), http.StatusInternalServerError)
		return
	}

	// Return the chunks as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chunks)
}

// handleGitIngest handles ingesting a Git repository
func handleGitIngest(w http.ResponseWriter, r *http.Request) {
	// Read query parameters
	cloneURL := r.URL.Query().Get("clone_url")
	if cloneURL == "" {
		http.Error(w, "clone_url is required", http.StatusBadRequest)
		return
	}

	// Read branch parameter (optional, defaults to empty string)
	branch := r.URL.Query().Get("branch")

	repoPath := "/tmp/git_repo" // Local path for cloning the repo
	privateKeyPath := ""        // Leave empty for public repositories

	// Create a GitLoader and pass in the DocumentManager and IndexManager
	gitLoader := documents.NewGitLoader(
		repoPath, cloneURL, branch, privateKeyPath,
		nil,   // Optional file filter, e.g., to include only specific files
		false, // Set to true if you want to skip host key verification
		docManager,
		indexManager,
	)

	// Load documents from the Git repository
	if err := gitLoader.Load(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to load Git repository: %s", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Git repository ingested and indexed successfully.")
}

// handlePDFIngest handles uploading and processing a PDF file
func handlePDFIngest(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse the uploaded file
	file, handler, err := r.FormFile("pdf")
	if err != nil {
		http.Error(w, "Error parsing uploaded file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save the uploaded file to a local directory
	savePath := filepath.Join("/tmp", handler.Filename)
	out, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Failed to save uploaded file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	// Copy file content to the new file
	_, err = io.Copy(out, file)
	if err != nil {
		http.Error(w, "Failed to save uploaded file", http.StatusInternalServerError)
		return
	}

	// Load the PDF into the DocumentManager
	pdfDoc, err := documents.LoadPDF(savePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to process PDF: %s", err), http.StatusInternalServerError)
		return
	}
	docManager.IngestDocument(pdfDoc)

	// Index the full document
	docID := pdfDoc.Metadata["file_path"] // or generate a unique ID
	if err := indexManager.IndexFullDocument(docID, pdfDoc.PageContent, pdfDoc.Metadata["file_path"]); err != nil {
		http.Error(w, "Failed to index full document", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "PDF ingested and indexed successfully.")
}

// handleSplitDocuments splits the content of all ingested documents and indexes them.
func handleSplitDocuments(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Starting document splitting process...")

	// Split the documents using the DocumentManager
	splits, err := docManager.SplitDocuments()
	if err != nil {
		fmt.Printf("Error: Failed to split documents: %s\n", err)
		http.Error(w, fmt.Sprintf("Failed to split documents: %s", err), http.StatusInternalServerError)
		return
	}

	// Provide feedback on the number of documents processed
	fmt.Printf("Document splitting complete. Processed %d documents.\n", len(splits))

	// Set the response header to plain text
	w.Header().Set("Content-Type", "text/plain")

	// Write each split chunk in the specified format
	for filePath, chunks := range splits {
		fmt.Printf("Document [%s]: Generated %d chunks.\n", filePath, len(chunks)) // Console output for each document
		for _, chunk := range chunks {
			// Pretty print the chunk
			prettyChunk := prettyPrintChunk(chunk)
			fmt.Fprintf(w, "[%s]\n%s\n\n", filePath, prettyChunk)
		}
	}

	fmt.Println("Document splitting and indexing operation completed.")
}

// prettyPrintChunk formats the text content for better readability.
func prettyPrintChunk(content string) string {
	// Check if the content appears to be HTML or XML and format accordingly
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		return prettyPrintHTML(content)
	}

	// For plain text, ensure line breaks and spaces are properly handled.
	return content
}

// prettyPrintHTML attempts to indent HTML for better readability.
func prettyPrintHTML(htmlContent string) string {
	// Use a basic approach to add new lines and indentations for common tags.
	replacer := strings.NewReplacer(
		"><", ">\n<", // Add new lines between tags
		"<div", "\n<div",
		"</div>", "</div>\n",
		"<p", "\n<p",
		"</p>", "</p>\n",
		"<li", "\n<li",
		"</li>", "</li>\n",
		"<ul", "\n<ul",
		"</ul>", "</ul>\n",
		"<optgroup", "\n<optgroup",
		"</optgroup>", "</optgroup>\n",
		"<select", "\n<select",
		"</select>", "</select>\n",
	)
	return replacer.Replace(htmlContent)
}

// isPrivateRepository is a helper function to determine if a repository is private
func isPrivateRepository(cloneURL string) bool {
	// Basic example: You can modify this to check if the URL requires authentication
	return false // Assume public by default; modify as needed to detect private repositories
}

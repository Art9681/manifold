// manifold/rag.go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	index "github.com/blevesearch/bleve_index_api"
	"github.com/labstack/echo/v4"
)

type RagRequest struct {
	Text string `json:"text"`
	TopN int    `json:"top_n"`
}

// handleQueryDocuments accepts a text input and returns a json object of similar documents with similarity score
func handleQueryDocuments(c echo.Context) error {
	// Get the request body
	req := new(RagRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	// Use IndexManager to create a search request based on input
	searchRequest := indexManager.CreateSearchRequest(req.Text, req.TopN)

	// Perform the search using IndexManager
	results, err := indexManager.SearchChunks(searchRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	// Prepare a response structure to hold results
	type SearchResult struct {
		ID       string  `json:"id"`
		Score    float64 `json:"score"`
		Prompt   string  `json:"prompt"`
		Response string  `json:"response"`
	}
	var searchResults []SearchResult

	// Iterate over the search hits and retrieve the prompt and response fields
	for _, hit := range results.Hits {
		doc, err := indexManager.GetDocument(hit.ID)
		if err != nil {
			fmt.Printf("Error retrieving document %s: %v\n", hit.ID, err)
			continue
		}

		var response string

		// Use a FieldVisitor from the index package
		doc.VisitFields(func(field index.Field) {
			fieldName := field.Name()
			fieldValue := string(field.Value())

			if fieldName == "chunk" {
				response += fieldValue + " "
			} else if fieldName == "full_content" {

				// print only the first 1000 characters of the full content
				log.Printf("Full content: %s", fieldValue[:1000])

				response += fieldValue
			}

		})

		// Append the search result to the response slice
		searchResults = append(searchResults, SearchResult{
			ID:       hit.ID,
			Score:    hit.Score,
			Prompt:   req.Text,
			Response: response,
		})
	}

	// Return the JSON response with the top N documents
	return c.JSON(http.StatusOK, searchResults)
}

func handleGitIngest(c echo.Context) error {
	if docManager == nil {
		return c.JSON(http.StatusInternalServerError, "DocumentManager is not initialized")
	}

	cloneURL := c.QueryParam("clone_url")
	if cloneURL == "" {
		return c.JSON(http.StatusBadRequest, "clone_url is required")
	}

	branch := c.QueryParam("branch")
	repoPath := "/tmp/git_repo"
	privateKeyPath := ""

	err := docManager.IngestGitRepo(repoPath, cloneURL, branch, privateKeyPath, nil, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("Failed to load Git repository: %s", err))
	}

	return c.String(http.StatusOK, "Git repository ingested and indexed successfully.")
}

// handlePDFIngest handles uploading and processing a PDF file
func handlePDFIngest(c echo.Context) error {
	// Only allow POST requests
	if c.Request().Method != http.MethodPost {
		return c.JSON(http.StatusMethodNotAllowed, "Invalid request method")
	}

	// Parse the uploaded file
	file, err := c.FormFile("pdf")
	if err != nil {
		return c.JSON(http.StatusBadRequest, "Error parsing uploaded file")
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer src.Close()

	savePath := filepath.Join("/tmp", file.Filename)
	dst, err := os.Create(savePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, "Failed to save uploaded file")
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, "Failed to save uploaded file")
	}

	err = docManager.IngestPDF(savePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("Failed to process PDF: %s", err))
	}

	return c.String(http.StatusOK, "PDF ingested and indexed successfully.")
}

// handleSplitDocuments splits the content of all ingested documents and indexes them.
func handleSplitDocuments(c echo.Context) error {
	fmt.Println("Starting document splitting process...")

	splits, err := docManager.SplitAndIndexDocuments()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("Failed to split documents: %s", err))
	}

	// Prepare a string response with details of the splits
	var result string
	for filePath, chunks := range splits {
		result += fmt.Sprintf("Document [%s]: Generated %d chunks.\n", filePath, len(chunks))
	}

	return c.String(http.StatusOK, result)
}

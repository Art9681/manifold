package main

import (
	"fmt"
	"net/http"

	"github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
	"github.com/labstack/echo/v4"
)

type RagRequest struct {
	Text string `json:"text"`
	TopN int    `json:"top_n"`
}

// handleRagRequest accepts a text input and returns a json object of similar documents with similarity score
func handleRagRequest(c echo.Context) error {
	// Get the request body
	req := new(RagRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	// Create a Bleve match query for the input text
	searchRequest := bleve.NewSearchRequest(bleve.NewMatchQuery(req.Text))
	searchRequest.Size = req.TopN // Limit results to the topN documents

	// Perform the search
	results, err := searchIndex.Search(searchRequest)
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
		doc, err := searchIndex.Document(hit.ID)
		if err != nil {
			fmt.Printf("Error retrieving document %s: %v\n", hit.ID, err)
			continue
		}

		var prompt, response string

		// Visit each field in the document and capture the "prompt" and "response" fields
		doc.VisitFields(func(field index.Field) {
			fieldName := field.Name()
			fieldValue := string(field.Value())

			if fieldName == "prompt" {
				prompt = fieldValue
			}
			if fieldName == "response" {
				response = fieldValue
			}
		})

		// Append the search result to the response slice
		searchResults = append(searchResults, SearchResult{
			ID:       hit.ID,
			Score:    hit.Score,
			Prompt:   prompt,
			Response: response,
		})
	}

	// Return the JSON response with the top 3 documents
	return c.JSON(http.StatusOK, searchResults)
}

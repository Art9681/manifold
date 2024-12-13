package main

import (
	"fmt"
	"log"

	"manifold/internal/web"
)

func main() {
	// 1. Test with a Single URL

	url := "https://blog.golang.org/go1.21"
	fmt.Println("===== Single URL Fetch =====")
	markdownContent, err := web.WebGetHandler(url)
	if err != nil {
		log.Printf("Error fetching URL %s: %v", url, err)
	} else {
		fmt.Println(markdownContent)
	}

	// 2. Test with DuckDuckGo Search

	searchQuery := "golang concurrency patterns"
	fmt.Println("\n===== DuckDuckGo Search Results =====")
	urls := web.SearchDDG(searchQuery)
	if urls == nil {
		log.Printf("Error fetching search results for %s", searchQuery)
	} else {
		searchResults := web.GetSearchResults(urls)
		fmt.Println(searchResults)
	}

	// 3. Test with SearXNG
	searxURL := "https://searx.tiekoetter.com/"
	searchQuerySearXNG := "rust async"
	fmt.Println("\n===== SearXNG Search Results =====")
	urlsSearXNG := web.GetSearXNGResults(searxURL, searchQuerySearXNG)
	if urlsSearXNG == nil {
		log.Printf("Error fetching searxng results for %s", searchQuerySearXNG)
	} else {
		searchResultsSearXNG := web.GetSearchResults(urlsSearXNG)
		fmt.Println(searchResultsSearXNG)
	}
}

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"manifold/internal/coderag"
)

func main() {
	// Initialize the CodeIndex
	index := coderag.NewCodeIndex()

	// Index the repository
	repoPath := "/Users/arturoaquino/Documents/manifold"
	fmt.Printf("Indexing repository at: %s\n", repoPath)
	if err := index.IndexRepository(repoPath); err != nil {
		log.Fatalf("Indexing failed: %v", err)
	}
	fmt.Println("Indexing completed successfully.\n")

	// Create a buffered reader for user input
	reader := bufio.NewReader(os.Stdin)

	for {
		// Prompt the user for input
		fmt.Print("Enter your query (or type 'exit' to quit): ")
		prompt, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading input: %v", err)
			continue
		}

		// Trim whitespace and check for exit condition
		prompt = strings.TrimSpace(prompt)
		if strings.EqualFold(prompt, "exit") {
			fmt.Println("Exiting the application.")
			break
		}

		// Handle the user prompt to retrieve relationships
		relationship, err := index.HandleUserPrompt(prompt)
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}

		// Display the relationships
		displayRelationshipInfo(relationship)
	}
}

// displayRelationshipInfo formats and prints the RelationshipInfo.
func displayRelationshipInfo(rel *coderag.RelationshipInfo) {
	fmt.Printf("\n=== Relationships for function '%s' ===\n\n", rel.FunctionName)

	// Display functions that this function calls
	if rel.TotalCalls > 0 {
		fmt.Printf("Functions it calls (%d):\n", rel.TotalCalls)
		for i, calledFunc := range rel.Calls {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, calledFunc, rel.CallsFilePaths[i])
		}
	} else {
		fmt.Printf("Functions it calls: None\n")
	}

	fmt.Println()

	// Display functions that call this function
	if rel.TotalCalledBy > 0 {
		fmt.Printf("Functions that call it (%d):\n", rel.TotalCalledBy)
		for i, callerFunc := range rel.CalledBy {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, callerFunc, rel.CalledByFilePaths[i])
		}
	} else {
		fmt.Printf("Functions that call it: None\n")
	}

	fmt.Println()
}

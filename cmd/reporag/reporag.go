// reporag.go
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
	// Load configuration for OpenAI API
	cfg, err := coderag.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Initialize the CodeIndex
	index := coderag.NewCodeIndex()

	// Index the repository
	repoPath := "E:\\manifold"
	fmt.Printf("Indexing repository at: %s\n", repoPath)
	if err := index.IndexRepository(repoPath, cfg); err != nil {
		log.Fatalf("Indexing failed: %v", err)
	}
	fmt.Println("Indexing completed successfully.")

	// Start the API server in a separate goroutine
	go index.StartAPIServer(8080)

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

		// Trim any extra spaces/newlines from user input
		prompt = strings.TrimSpace(prompt)

		// Exit if the user types "exit"
		if strings.ToLower(prompt) == "exit" {
			fmt.Println("Exiting the application. Goodbye!")
			break
		}

		// Handle special commands like "refactor" to show refactoring opportunities
		if strings.ToLower(prompt) == "refactor" {
			displayRefactoringOpportunities(index)
			continue
		}

		// Handle the user's prompt to query a function
		relationshipInfo, err := index.HandleUserPrompt(prompt)
		if err != nil {
			fmt.Printf("Error processing query: %v\n", err)
			continue
		}

		// Display the relationship information, comments, and source code
		displayRelationshipInfo(relationshipInfo)
	}
}

// displayRelationshipInfo displays detailed information about a function or method.
func displayRelationshipInfo(info *coderag.RelationshipInfo) {
	fmt.Printf("\nFunction: %s\n", info.FunctionName)
	fmt.Printf("Total Calls: %d\n", info.TotalCalls)
	fmt.Printf("Total Called By: %d\n", info.TotalCalledBy)

	// Display function comments if available
	if info.Comments != "" {
		fmt.Printf("\nComments:\n%s\n", info.Comments)
	} else {
		fmt.Println("\nComments: None")
	}

	// Display the summary if available
	if info.Summary != "" {
		fmt.Printf("\nSummary:\n%s\n", info.Summary)
	} else {
		fmt.Println("\nSummary: None")
	}

	// Display the source code of the function
	fmt.Printf("\nSource Code:\n%s\n", info.Code)

	// Display the functions this function calls
	fmt.Printf("\nFunctions Called by %s:\n", info.FunctionName)
	if len(info.Calls) > 0 {
		for i, calledFunc := range info.Calls {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, calledFunc, info.CallsFilePaths[i])
		}
	} else {
		fmt.Println("  None")
	}

	// Display the functions that call this function
	fmt.Printf("\nFunctions Calling %s:\n", info.FunctionName)
	if len(info.CalledBy) > 0 {
		for i, callerFunc := range info.CalledBy {
			fmt.Printf("  %d. %s (File: %s)\n", i+1, callerFunc, info.CalledByFilePaths[i])
		}
	} else {
		fmt.Println("  None")
	}
	fmt.Println()
}

// displayRefactoringOpportunities displays potential refactoring opportunities found in the codebase.
func displayRefactoringOpportunities(index *coderag.CodeIndex) {
	fmt.Println("\nRefactoring Opportunities:")
	if len(index.RefactoringOpportunities) == 0 {
		fmt.Println("No refactoring opportunities found.")
		return
	}

	for i, opp := range index.RefactoringOpportunities {
		fmt.Printf("%d. %s\n   Location: %s\n   Severity: %s\n\n", i+1, opp.Description, opp.Location, opp.Severity)
	}
}

package main

import (
	"fmt"
	"log"

	"manifold/internal/coderag"
)

func main() {
	// Initialize the CodeIndex
	index := coderag.NewCodeIndex()

	// Index the repository
	repoPath := "/Users/arturoaquino/Documents/manifold"
	if err := index.IndexRepository(repoPath); err != nil {
		log.Fatalf("Indexing failed: %v", err)
	}

	// Retrieve information about a specific function
	funcName := "SaveChatTurn"
	funcInfo, err := index.GetFunctionInfo(funcName)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("Function: %s defined in %s:%d\n", funcInfo.Name, funcInfo.FilePath, funcInfo.LineNumber)
	}

	// Get related functions
	relatedFuncs, err := index.GetRelatedFunctions(funcName)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("Functions related to %s:\n", funcName)
		for _, f := range relatedFuncs {
			fmt.Printf("- %s defined in %s:%d\n", f.Name, f.FilePath, f.LineNumber)
		}
	}

	// Print the call tree for the function
	fmt.Printf("Call tree for %s:\n", funcName)
	index.PrintCallTree(funcName, 0, nil)
}

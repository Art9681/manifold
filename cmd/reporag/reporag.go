package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"manifold/internal/coderag"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Constants for database
const (
	dbPath               = "/Users/arturoaquino/.manifold/datasets/eternaldata.db" // Path to SQLite DB
	chatFTSTableCreation = `
		CREATE VIRTUAL TABLE IF NOT EXISTS chat_fts USING fts5(
			prompt,
			response,
			modelName,
			tokenize = "porter"
		);
	`
)

// SQLiteDB structure to hold the *sql.DB object
type SQLiteDB struct {
	db *sql.DB
}

// Initialize the SQLite database, create FTS5 tables if they don't exist
func initializeDatabase(dbPath string) (*SQLiteDB, error) {
	// Open the SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %v", err)
	}

	// Test the database connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping SQLite database: %v", err)
	}

	// Create FTS5 table if it doesn't exist
	_, err = db.Exec(chatFTSTableCreation)
	if err != nil {
		return nil, fmt.Errorf("failed to create FTS5 table: %v", err)
	}

	log.Println("SQLite database initialized and FTS5 table ensured.")
	return &SQLiteDB{db: db}, nil
}

// InsertChunk inserts a chunk and its summary into the FTS5 table
func (sqldb *SQLiteDB) InsertChunk(ctx context.Context, summary string, chunk string) error {
	query := `
		INSERT INTO chat_fts(prompt, response, modelName)
		VALUES (?, ?, 'assistant');
	`
	_, err := sqldb.db.ExecContext(ctx, query, summary, chunk)
	if err != nil {
		return fmt.Errorf("failed to insert into FTS5: %v", err)
	}
	return nil
}

// QueryChunks searches the FTS5 table for relevant chunks based on the user query
func (sqldb *SQLiteDB) QueryChunks(ctx context.Context, userQuery string) ([]string, error) {
	query := `
		SELECT response FROM chat_fts
		WHERE chat_fts MATCH ?
		LIMIT 10;
	`
	rows, err := sqldb.db.QueryContext(ctx, query, userQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query FTS5 table: %v", err)
	}
	defer rows.Close()

	var chunks []string
	for rows.Next() {
		var chunk string
		if err := rows.Scan(&chunk); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %v", err)
		}
		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %v", err)
	}

	return chunks, nil
}

// insertChunksIntoDB processes the indexed repository and inserts chunks into the SQLite FTS5 database
func insertChunksIntoDB(ctx context.Context, sqldb *SQLiteDB, index *coderag.CodeIndex) error {
	summaries, chunks := index.GetChunksAndSummaries()

	if len(chunks) != len(summaries) {
		return fmt.Errorf("mismatch between number of chunks and summaries")
	}

	tx, err := sqldb.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO chat_fts(prompt, response, modelName)
		VALUES (?, ?, 'assistant');
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	var wg sync.WaitGroup
	for i := 0; i < len(chunks); i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := stmt.ExecContext(ctx, summaries[i], chunks[i])
			if err != nil {
				log.Printf("Failed to insert chunk %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	log.Println("All chunks have been inserted into the SQLite FTS5 database.")
	return nil
}

func main() {
	// Initialize the SQLite database
	sqldb, err := initializeDatabase(dbPath)
	if err != nil {
		log.Fatalf("Database initialization error: %v", err)
	}
	defer sqldb.db.Close()

	// Load configuration for OpenAI API
	cfg, err := coderag.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Initialize the CodeIndex
	index := coderag.NewCodeIndex()

	// Channel to handle repository indexing
	indexingChan := make(chan struct{})
	var wg sync.WaitGroup

	// Start indexing repository in a separate goroutine with WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		repoPath := "/Users/arturoaquino/Documents/manifold"
		fmt.Printf("Indexing repository at: %s\n", repoPath)
		if err := index.IndexRepository(repoPath, cfg); err != nil {
			log.Fatalf("Indexing failed: %v", err)
		}
		fmt.Println("Indexing completed successfully.")
		indexingChan <- struct{}{} // Notify that indexing is done

		// After indexing, insert chunks into the SQLite FTS5 database
		err := insertChunksIntoDB(context.Background(), sqldb, index)
		if err != nil {
			log.Fatalf("Failed to insert chunks into database: %v", err)
		}
	}()

	// Start the API server in a separate goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		index.StartAPIServer(8080)
	}()

	// Create a buffered reader for user input
	reader := bufio.NewReader(os.Stdin)

	// Wait for repository indexing to complete
	go func() {
		<-indexingChan
		fmt.Println("Repository indexing is completed and chunks are stored in the database. Ready to handle queries.")
	}()

	// Channel to handle user queries concurrently
	queryChan := make(chan string)

	// Start a worker pool to handle user queries concurrently
	numWorkers := 4
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(queryChan, index, sqldb, &wg)
	}

	// Main loop to read user input and send to query workers
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
			close(queryChan) // Signal workers to stop
			break
		}

		// Handle special commands like "refactor" to show refactoring opportunities
		if strings.ToLower(prompt) == "refactor" {
			displayRefactoringOpportunities(index)
			continue
		}

		// Send user prompt to worker goroutines
		queryChan <- prompt
	}

	// Wait for all goroutines to finish before exiting
	wg.Wait()
}

// Worker function to handle queries concurrently
func worker(queryChan <-chan string, index *coderag.CodeIndex, sqldb *SQLiteDB, wg *sync.WaitGroup) {
	defer wg.Done()
	for prompt := range queryChan {
		// Query the SQLite FTS5 database for relevant chunks
		chunks, err := sqldb.QueryChunks(context.Background(), prompt)
		if err != nil {
			fmt.Printf("Error querying database: %v\n", err)
			continue
		}

		if len(chunks) == 0 {
			fmt.Println("No relevant chunks found for your query.")
			continue
		}

		// Optionally, process the chunks using CodeIndex or other logic
		// For demonstration, we'll just display the chunks
		fmt.Printf("\nRelevant Chunks for your query:\n")
		for i, chunk := range chunks {
			fmt.Printf("Chunk %d:\n%s\n\n", i+1, chunk)
		}
	}
}

// displayRelationshipInfo displays detailed information about a function or method.
// (Assuming this function remains unchanged)
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

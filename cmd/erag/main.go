package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// Constants
const (
	embeddingDim         = 768         // Dimension of the embeddings
	vecTableName         = "vec_items" // Name of the vector table
	vecTableCreationStmt = `CREATE VIRTUAL TABLE IF NOT EXISTS vec_items USING vec0(embedding float[768]);`
)

// Chat represents an entry in the regular "chats" table.
type Chat struct {
	ID        int64  `json:"id"`
	Prompt    string `json:"prompt"`
	Response  string `json:"response"`
	ModelName string `json:"modelName"`
	// Embedding is no longer stored here
}

// ChatEntry represents an entry in the FTS5 table with both prompt and response.
type ChatEntry struct {
	Prompt   string `json:"prompt"`   // Corresponds to the passage from Parquet
	Response string `json:"response"` // Initialized as empty
}

// SQLiteDB structure to hold the *sql.DB object
type SQLiteDB struct {
	db *sql.DB
}

// EmbeddingRequest represents the JSON structure sent to the embeddings endpoint.
type EmbeddingRequest struct {
	Input []string `json:"input"`
}

// Embedding represents a single embedding data point.
type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// UsageMetrics details the token usage of the Embeddings API request.
type UsageMetrics struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingResponse represents the expected JSON structure received from the embeddings endpoint.
type EmbeddingResponse struct {
	Object string       `json:"object"`
	Data   []Embedding  `json:"data"`
	Model  string       `json:"model"`
	Usage  UsageMetrics `json:"usage"`
}

// Initialize a single http.Client to reuse connections
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// Initialize the embeddings server URL from environment variable or use default
var embeddingsURL = func() string {
	url := os.Getenv("EMBEDDINGS_URL")
	if url == "" {
		url = "http://192.168.0.110:32184/embeddings"
	}
	return url
}()

// initializeDatabase initializes the SQLite database, registers sqlite-vec, and creates necessary tables.
func initializeDatabase(dataPath string) (*SQLiteDB, error) {
	// Register the sqlite-vec extension
	sqlite_vec.Auto()

	dbPath := filepath.Join(dataPath, "eternaldata.db")
	dbExists := fileExists(dbPath)

	// Open the SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %v", err)
	}

	// Test the database connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping SQLite database: %v", err)
	}

	// If the database doesn't exist, create tables
	if !dbExists {
		log.Println("Database does not exist. Creating new database and tables.")

		// Begin a transaction
		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("failed to begin transaction: %v", err)
		}

		// Create the regular "chats" table without the embedding column
		createChatsTable := `
			CREATE TABLE IF NOT EXISTS chats (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				prompt TEXT NOT NULL,
				response TEXT,
				modelName TEXT
			);
		`
		_, err = tx.Exec(createChatsTable)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create chats table: %v", err)
		}

		// Create the FTS5 virtual table for full-text search
		createFTSTable := `
			CREATE VIRTUAL TABLE IF NOT EXISTS chat_fts USING fts5(
				prompt,
				response,
				tokenize = "porter"
			);
		`
		_, err = tx.Exec(createFTSTable)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create FTS5 table: %v", err)
		}

		// Create the vec_items virtual table for vector embeddings
		_, err = tx.Exec(vecTableCreationStmt)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create vec_items table: %v", err)
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %v", err)
		}

		log.Println("Database and tables created successfully.")
	} else {
		log.Println("Existing database found.")
	}

	return &SQLiteDB{db: db}, nil
}

// fileExists checks if a file exists at the given path.
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// InsertChatEntry inserts a chat entry into the FTS5 table.
func (sqldb *SQLiteDB) InsertChatEntry(ctx context.Context, entry ChatEntry) error {
	query := `
		INSERT INTO chat_fts(prompt, response)
		VALUES (?, ?);
	`
	_, err := sqldb.db.ExecContext(ctx, query, entry.Prompt, entry.Response)
	if err != nil {
		return fmt.Errorf("failed to insert into FTS5: %v", err)
	}
	return nil
}

// UpsertChat upserts a chat entry into the regular "chats" table and inserts the embedding into vec_items.
func (sqldb *SQLiteDB) UpsertChat(ctx context.Context, chat *Chat, embedding []float32) error {
	var tx *sql.Tx
	var err error

	// Start a transaction
	tx, err = sqldb.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Insert or update the chats table
	if chat.ID == 0 {
		// Insert new chat
		insertChatQuery := `
			INSERT INTO chats (prompt, response, modelName)
			VALUES (?, ?, ?);
		`
		result, err := tx.ExecContext(ctx, insertChatQuery, chat.Prompt, chat.Response, chat.ModelName)
		if err != nil {
			return fmt.Errorf("failed to insert into chats table: %v", err)
		}
		chat.ID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to retrieve last insert ID: %v", err)
		}
	} else {
		// Update existing chat
		updateChatQuery := `
			UPDATE chats
			SET prompt = ?, response = ?, modelName = ?
			WHERE id = ?;
		`
		_, err = tx.ExecContext(ctx, updateChatQuery, chat.Prompt, chat.Response, chat.ModelName, chat.ID)
		if err != nil {
			return fmt.Errorf("failed to update chats table: %v", err)
		}
	}

	// Serialize the embedding
	serializedEmbedding, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("failed to serialize embedding: %v", err)
	}

	// Insert or replace the embedding in vec_items using REPLACE INTO
	insertVecQuery := `
		REPLACE INTO vec_items(rowid, embedding)
		VALUES (?, ?);
	`
	_, err = tx.ExecContext(ctx, insertVecQuery, chat.ID, serializedEmbedding)
	if err != nil {
		return fmt.Errorf("failed to upsert into vec_items table: %v", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

// fetchEmbeddings sends a batch POST request to the embeddings endpoint and retrieves the embeddings.
func fetchEmbeddings(ctx context.Context, prompts []string) ([][]float32, error) {
	var embeddings [][]float32
	var retries = 3
	var maxBackoff = 30 * time.Second

	for attempt := 1; attempt <= retries; attempt++ {
		// Create the request payload
		reqBody := EmbeddingRequest{
			Input: prompts,
		}
		reqBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal embedding request: %v", err)
		}

		// Create the HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", embeddingsURL, bytes.NewBuffer(reqBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request for embedding: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request using the shared http.Client
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Attempt %d: failed to send HTTP request for embedding: %v", attempt, err)
			// Exponential backoff with cap
			sleepDuration := time.Duration(attempt*attempt) * time.Second
			if sleepDuration > maxBackoff {
				sleepDuration = maxBackoff
			}
			time.Sleep(sleepDuration + time.Duration(rand.Intn(1000))*time.Millisecond)
			continue
		}

		// Read and parse the response
		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Attempt %d: failed to read embedding response: %v", attempt, err)
			// Exponential backoff with cap
			sleepDuration := time.Duration(attempt*attempt) * time.Second
			if sleepDuration > maxBackoff {
				sleepDuration = maxBackoff
			}
			time.Sleep(sleepDuration + time.Duration(rand.Intn(1000))*time.Millisecond)
			continue
		}

		// Check for non-200 status codes
		if resp.StatusCode != http.StatusOK {
			log.Printf("Attempt %d: non-OK HTTP status: %s, body: %s", attempt, resp.Status, string(respBytes))
			// Exponential backoff with cap
			sleepDuration := time.Duration(attempt*attempt) * time.Second
			if sleepDuration > maxBackoff {
				sleepDuration = maxBackoff
			}
			time.Sleep(sleepDuration + time.Duration(rand.Intn(1000))*time.Millisecond)
			continue
		}

		// Unmarshal the response
		var embeddingResp EmbeddingResponse
		err = json.Unmarshal(respBytes, &embeddingResp)
		if err != nil {
			log.Printf("Attempt %d: failed to unmarshal embedding response: %v", attempt, err)
			// Exponential backoff with cap
			sleepDuration := time.Duration(attempt*attempt) * time.Second
			if sleepDuration > maxBackoff {
				sleepDuration = maxBackoff
			}
			time.Sleep(sleepDuration + time.Duration(rand.Intn(1000))*time.Millisecond)
			continue
		}

		// Ensure we have embeddings in the response
		if len(embeddingResp.Data) == 0 {
			log.Printf("Attempt %d: no embedding data returned in response", attempt)
			// Exponential backoff with cap
			sleepDuration := time.Duration(attempt*attempt) * time.Second
			if sleepDuration > maxBackoff {
				sleepDuration = maxBackoff
			}
			time.Sleep(sleepDuration + time.Duration(rand.Intn(1000))*time.Millisecond)
			continue
		}

		// Convert embeddings from []float64 to []float32
		embeddings = make([][]float32, len(embeddingResp.Data))
		for i, data := range embeddingResp.Data {
			if len(data.Embedding) != embeddingDim {
				log.Printf("Embedding dimension mismatch for index %d: expected %d, got %d", i, embeddingDim, len(data.Embedding))
				return nil, fmt.Errorf("embedding dimension mismatch at index %d", i)
			}
			embeddings[i] = make([]float32, embeddingDim)
			for j, val := range data.Embedding {
				embeddings[i][j] = float32(val)
			}
		}
		return embeddings, nil
	}

	return nil, fmt.Errorf("failed to fetch embeddings after %d attempts", retries)
}

// LoadParquetData loads data from the Parquet file, generates embeddings, and inserts data into FTS5, chats, and vec_items tables.
func (sqldb *SQLiteDB) LoadParquetData(ctx context.Context, parquetFilePath string) error {
	// Open the Parquet file
	fr, err := local.NewLocalFileReader(parquetFilePath)
	if err != nil {
		return fmt.Errorf("failed to open parquet file: %v", err)
	}
	defer fr.Close()

	// Create a new Parquet reader
	pr, err := reader.NewParquetReader(fr, nil, 4)
	if err != nil {
		return fmt.Errorf("can't create parquet reader: %v", err)
	}
	defer pr.ReadStop()

	// Get the number of rows in the Parquet file
	numRows := int(pr.GetNumRows())

	// Read all rows into a slice
	rows, err := pr.ReadByNumber(numRows)
	if err != nil {
		return fmt.Errorf("can't read parquet file: %v", err)
	}

	// Convert the rows to JSON
	jsonBs, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("failed to convert parquet data to JSON: %v", err)
	}

	// Log the JSON for debugging (optional)
	log.Println("JSON Data from Parquet:", string(jsonBs))

	// Unmarshal JSON data into ChatEntry structs with empty responses
	var rawPassages []map[string]interface{}
	err = json.Unmarshal(jsonBs, &rawPassages)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON data: %v", err)
	}

	var chatEntries []ChatEntry
	var chats []*Chat
	for _, raw := range rawPassages {
		// Assuming the Parquet data has a field named "Passage"
		prompt, ok := raw["Passage"].(string)
		if !ok {
			log.Printf("Invalid passage format: %v", raw["Passage"])
			continue
		}

		// Create the ChatEntry for the FTS5 table
		chatEntries = append(chatEntries, ChatEntry{
			Prompt:   prompt,
			Response: "", // Initialize response as an empty string
		})

		// Initialize the Chat struct with default values
		chats = append(chats, &Chat{
			Prompt:    prompt,
			Response:  "",              // Initialize response as an empty string
			ModelName: "default_model", // Default model name
		})
	}

	// Process embeddings in chunks of 10 with concurrency control
	chunkSize := 10
	maxConcurrentWorkers := 3
	sem := make(chan struct{}, maxConcurrentWorkers)
	var wg sync.WaitGroup

	for i := 0; i < len(chatEntries); i += chunkSize {
		end := i + chunkSize
		if end > len(chatEntries) {
			end = len(chatEntries)
		}

		// Extract the prompts for the current chunk
		chunkPrompts := make([]string, end-i)
		for j := i; j < end; j++ {
			chunkPrompts[j-i] = chatEntries[j].Prompt
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(start, end int, prompts []string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Fetch embeddings for the chunk
			embeddings, err := fetchEmbeddings(ctx, prompts)
			if err != nil {
				log.Printf("Failed to fetch embeddings for chunk starting at %d: %v. Skipping chunk.", start, err)
				return
			}

			// Update Chat entries with embeddings
			for j := start; j < end; j++ {
				embedding := embeddings[j-start]

				// Insert the chat entry into the FTS5 table
				err = sqldb.InsertChatEntry(ctx, chatEntries[j])
				if err != nil {
					log.Printf("Failed to insert chat entry into FTS5 for prompt ID %d: %v. Skipping entry.", j, err)
					continue
				}

				// Upsert the Chat entry into the regular "chats" table and insert into vec_items
				err = sqldb.UpsertChat(ctx, chats[j], embedding)
				if err != nil {
					log.Printf("Failed to upsert chat entry and embedding for prompt ID %d: %v. Skipping entry.", j, err)
					continue
				}
			}
		}(i, end, chunkPrompts)
	}

	wg.Wait()

	log.Println("Parquet data with embeddings inserted successfully into FTS5, chats, and vec_items tables.")
	return nil
}

func main() {
	// Define the data path where the SQLite database will be stored
	dataPath := "/Users/arturoaquino/.manifold/datasets"

	// Define the path to the Parquet file
	parquetFilePath := "/Users/arturoaquino/.manifold/datasets/part.0.parquet"

	// Initialize the SQLite database
	db, err := initializeDatabase(dataPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.db.Close()

	// Load and insert data from the Parquet file into the SQLite database
	ctx := context.Background()
	err = db.LoadParquetData(ctx, parquetFilePath)
	if err != nil {
		log.Fatalf("Failed to load data from Parquet file: %v", err)
	}

	log.Println("Database setup and Parquet data insertion complete")
}

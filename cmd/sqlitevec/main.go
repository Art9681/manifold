package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// generateMockEmbedding generates a random embedding with the given dimensions
func generateMockEmbedding(dimensions int) []float64 {
	rand.Seed(time.Now().UnixNano())
	embedding := make([]float64, dimensions)
	for i := 0; i < dimensions; i++ {
		embedding[i] = rand.Float64()*2 - 1 // Generates values between -1 and 1
	}
	return embedding
}

// embeddingToBlob converts a slice of float64 to a byte slice for storing in SQLite BLOB
func embeddingToBlob(embedding []float64) []byte {
	buf := make([]byte, len(embedding)*8) // Each float64 is 8 bytes
	for i, v := range embedding {
		binary.LittleEndian.PutUint64(buf[i*8:(i+1)*8], math.Float64bits(v))
	}
	return buf
}

func main() {
	// Open an in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get a dedicated connection from the pool
	conn, err := db.Conn(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Enable the FTS5 extension
	_, err = conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON;`)
	if err != nil {
		log.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Create a virtual table using FTS5 for full-text search on 'phrase'
	_, err = conn.ExecContext(context.Background(), `
		CREATE VIRTUAL TABLE phrases_fts USING fts5(phrase);
	`)
	if err != nil {
		log.Fatalf("Failed to create FTS5 table: %v", err)
	}

	// Create a table to store the phrase and its embedding
	_, err = conn.ExecContext(context.Background(), `
		CREATE TABLE vectors (
			id INTEGER PRIMARY KEY,
			phrase TEXT,
			embedding BLOB
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create vectors table: %v", err)
	}

	// Simulate embedding generation for the phrase "I love you"
	phrase := "I love you"
	dimensions := 384 // Example embedding size
	embedding := generateMockEmbedding(dimensions)

	// Convert the embedding to a byte slice for storage in SQLite
	embeddingBlob := embeddingToBlob(embedding)

	// Insert the phrase and its embedding into the SQLite database
	_, err = conn.ExecContext(context.Background(), `
		INSERT INTO vectors (phrase, embedding)
		VALUES (?, ?);
	`, phrase, embeddingBlob)
	if err != nil {
		log.Fatalf("Failed to insert embedding: %v", err)
	}

	// Insert the phrase into the FTS5 table for full-text search
	_, err = conn.ExecContext(context.Background(), `
		INSERT INTO phrases_fts (phrase)
		VALUES (?);
	`, phrase)
	if err != nil {
		log.Fatalf("Failed to insert phrase into FTS5 table: %v", err)
	}

	// Perform a full-text search for the phrase "love"
	rows, err := conn.QueryContext(context.Background(), `
		SELECT phrase FROM phrases_fts WHERE phrase MATCH ?;
	`, "love")
	if err != nil {
		log.Fatalf("Failed to perform FTS5 search: %v", err)
	}
	defer rows.Close()

	// Log the results of the full-text search
	for rows.Next() {
		var matchedPhrase string
		if err := rows.Scan(&matchedPhrase); err != nil {
			log.Fatalf("Failed to scan result: %v", err)
		}
		log.Printf("Full-Text Search Result: %s\n", matchedPhrase)
	}

	// Query and retrieve the stored embedding
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var retrievedPhrase string
		var retrievedEmbedding []byte
		err = conn.QueryRowContext(context.Background(), `
			SELECT phrase, embedding
			FROM vectors
			WHERE phrase = ?;
		`, phrase).Scan(&retrievedPhrase, &retrievedEmbedding)
		if err != nil {
			log.Fatalf("Failed to query embedding: %v", err)
		}

		// Convert the retrieved embedding back to a float64 slice
		retrievedEmbeddingFloats := make([]float64, len(retrievedEmbedding)/8)
		for i := 0; i < len(retrievedEmbeddingFloats); i++ {
			retrievedEmbeddingFloats[i] = math.Float64frombits(binary.LittleEndian.Uint64(retrievedEmbedding[i*8:]))
		}

		// Log the retrieved phrase and embedding
		log.Printf("Phrase: %s\n", retrievedPhrase)
		log.Printf("Retrieved Embedding: %v\n", retrievedEmbeddingFloats)
	}()
	wg.Wait()
}

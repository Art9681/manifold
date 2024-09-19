package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"
)

// Vector represents a vector of floats.
type Vector []float64

// Node is a struct that represents a node in a k-d tree.
type Node struct {
	Domain []float64
	Value  float64
	Left   *Node
	Right  *Node
}

// Embedding represents a word embedding.
type Embeddings struct {
	Word       string
	Vector     []float64
	Similarity float64 // Similarity field to store the cosine similarity
}

// EmbeddingDB represents a database of Embeddings.
type EmbeddingDB struct {
	Embeddings map[string]Embeddings
}

// Document represents a document to be ranked.
type Document struct {
	ID     string
	Score  float64
	Length int
}

// NewEmbeddingDB creates a new embedding database.
func NewEmbeddingDB() *EmbeddingDB {
	return &EmbeddingDB{
		Embeddings: make(map[string]Embeddings),
	}
}

// AddEmbedding adds a new embedding to the database.
func (db *EmbeddingDB) AddEmbedding(embedding Embeddings) {
	db.Embeddings[embedding.Word] = embedding
}

// AddEmbeddings adds a slice of embeddings to the database.
func (db *EmbeddingDB) AddEmbeddings(embeddings []Embeddings) {
	for _, embedding := range embeddings {
		db.AddEmbedding(embedding)
	}
}

// GenerateEmbedding generates vectors for each chunk of text by calling the LLMClient's SendEmbeddingRequest.
func GenerateEmbeddings(textChunks []string, client LLMClient) ([]Embeddings, error) {
	// Create an embedding request payload
	payload := &EmbeddingRequest{
		Input: textChunks,
		Model: "nomic-embed-text-v1.5.Q8_0.gguf",
	}

	// Send embedding request through the LLMClient
	resp, err := client.SendEmbeddingRequest(payload)
	if err != nil {
		return nil, fmt.Errorf("error sending embedding request: %v", err)
	}
	defer resp.Body.Close()

	// Check the response code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("received non-200 response code: %v", resp.StatusCode)
	}

	// Decode the response body
	var embeddingResponse EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	// Build the list of Embedding objects
	var embeddings []Embeddings
	for i, data := range embeddingResponse.Data {
		embeddings = append(embeddings, Embeddings{
			Word:   textChunks[i],
			Vector: data.Embedding,
		})
	}

	return embeddings, nil
}

// SaveEmbeddings saves the Embeddings to a file, appending new ones to existing data.
func (db *EmbeddingDB) SaveEmbeddings(path string) error {
	// Read the existing content from the file
	var existingEmbeddings map[string]Embeddings
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error reading file: %v", err)
		}
		existingEmbeddings = make(map[string]Embeddings)
	} else {
		err = json.Unmarshal(content, &existingEmbeddings)
		if err != nil {
			return fmt.Errorf("error unmarshaling existing embeddings: %v", err)
		}
	}

	// Merge new embeddings with existing ones
	for key, embedding := range db.Embeddings {
		existingEmbeddings[key] = embedding
	}

	// Marshal the combined embeddings to JSON
	jsonData, err := json.Marshal(existingEmbeddings)
	if err != nil {
		return fmt.Errorf("error marshaling embeddings: %v", err)
	}

	// Open the file in write mode (this will overwrite the existing file)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	defer f.Close()

	// Write the combined JSON to the file
	if _, err := f.Write(jsonData); err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
}

// LoadEmbeddings loads the Embeddings from a file.
func (db *EmbeddingDB) LoadEmbeddings(path string) (map[string]Embeddings, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var embeddings map[string]Embeddings
	err = json.Unmarshal(content, &embeddings)
	if err != nil {
		return nil, err
	}

	return embeddings, nil
}

// RetrieveEmbedding retrieves an embedding from the database.
func (db *EmbeddingDB) RetrieveEmbedding(word string) ([]float64, bool) {
	embedding, exists := db.Embeddings[word]
	if !exists {
		return nil, false
	}

	return embedding.Vector, true
}

// CosineSimilarity calculates the cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		log.Fatal("Vectors must be of the same length")
	}

	var dotProduct, magnitudeA, magnitudeB float64
	var wg sync.WaitGroup

	partitions := runtime.NumCPU()
	partSize := len(a) / partitions

	results := make([]struct {
		dotProduct, magnitudeA, magnitudeB float64
	}, partitions)

	for i := 0; i < partitions; i++ {
		wg.Add(1)
		go func(partition int) {
			defer wg.Done()
			start := partition * partSize
			end := start + partSize
			if partition == partitions-1 {
				end = len(a)
			}
			for j := start; j < end; j++ {
				results[partition].dotProduct += a[j] * b[j]
				results[partition].magnitudeA += a[j] * a[j]
				results[partition].magnitudeB += b[j] * b[j]
			}
		}(i)
	}

	wg.Wait()

	for _, result := range results {
		dotProduct += result.dotProduct
		magnitudeA += result.magnitudeA
		magnitudeB += result.magnitudeB
	}

	// Directly return cosine similarity without normalization
	return dotProduct / (math.Sqrt(magnitudeA) * math.Sqrt(magnitudeB))
}

// NormalizeL2 normalizes a vector using L2 normalization.
func NormalizeL2(vec []float64) []float64 {
	var sumSquares float64
	for _, value := range vec {
		sumSquares += value * value
	}
	norm := math.Sqrt(sumSquares)
	for i, value := range vec {
		vec[i] = value / norm
	}
	return vec
}

// SortEmbeddingsBySimilarity sorts a slice of Embeddings by similarity in descending order.
func SortEmbeddingsBySimilarity(embeddings []Embeddings) {
	sort.Slice(embeddings, func(i, j int) bool {
		return embeddings[i].Similarity > embeddings[j].Similarity
	})
}

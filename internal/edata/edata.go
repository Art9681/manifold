package edata

import (
	"bytes"
	"encoding/gob"
	"errors"
	"math"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Global database variable
var db *gorm.DB
var dbOnce sync.Once

// Initialize the database connection and migrate the schema
func InitDB(dbName string) error {
	var err error
	dbOnce.Do(func() {
		db, err = gorm.Open(sqlite.Open(dbName), &gorm.Config{})
		if err != nil {
			return
		}
		// Migrate the schema
		err = db.AutoMigrate(&Document{}, &Embedding{}, &GraphNode{}, &GraphEdge{})
	})
	return err
}

// Document model stores the text documents
type Document struct {
	ID   uint   `gorm:"primaryKey"`
	Text string `gorm:"type:text"`
}

// Embedding model stores the vector embeddings associated with documents
type Embedding struct {
	ID         uint      `gorm:"primaryKey"`
	DocumentID uint      `gorm:"index"`
	Vector     []float64 `gorm:"-"`
	VectorBlob []byte    `gorm:"type:blob"`
}

// GraphNode represents a node in the graph, linked to a document
type GraphNode struct {
	ID         uint `gorm:"primaryKey"`
	DocumentID uint `gorm:"index"`
}

// GraphEdge represents an edge between two graph nodes
type GraphEdge struct {
	ID     uint `gorm:"primaryKey"`
	FromID uint `gorm:"index"`
	ToID   uint `gorm:"index"`
}

// BeforeSave hooks into GORM's save process to serialize the vector
func (e *Embedding) BeforeSave(tx *gorm.DB) (err error) {
	e.VectorBlob, err = serializeVector(e.Vector)
	return
}

// AfterFind hooks into GORM's query process to deserialize the vector
func (e *Embedding) AfterFind(tx *gorm.DB) (err error) {
	e.Vector, err = deserializeVector(e.VectorBlob)
	return
}

// Serialize the vector into bytes using encoding/gob
func serializeVector(v []float64) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	return buf.Bytes(), err
}

// Deserialize the vector from bytes using encoding/gob
func deserializeVector(data []byte) ([]float64, error) {
	var v []float64
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&v)
	return v, err
}

// SaveDocument stores a text document and its vector embedding
func SaveDocument(text string, vector []float64) (*Document, error) {
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	doc := Document{Text: text}
	if err := db.Create(&doc).Error; err != nil {
		return nil, err
	}

	// Save the embedding if provided
	if vector != nil {
		embedding := Embedding{
			DocumentID: doc.ID,
			Vector:     vector,
		}
		if err := db.Create(&embedding).Error; err != nil {
			return &doc, err
		}
	}

	return &doc, nil
}

// AddGraphEdge adds an edge between two documents in the graph
func AddGraphEdge(fromDocID, toDocID uint) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	fromNode, err := getOrCreateGraphNode(fromDocID)
	if err != nil {
		return err
	}
	toNode, err := getOrCreateGraphNode(toDocID)
	if err != nil {
		return err
	}

	edge := GraphEdge{
		FromID: fromNode.ID,
		ToID:   toNode.ID,
	}
	return db.Create(&edge).Error
}

// Helper function to get or create a graph node for a document
func getOrCreateGraphNode(docID uint) (GraphNode, error) {
	var node GraphNode
	result := db.First(&node, "document_id = ?", docID)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		node = GraphNode{DocumentID: docID}
		if err := db.Create(&node).Error; err != nil {
			return node, err
		}
	} else if result.Error != nil {
		return node, result.Error
	}
	return node, nil
}

// GetSimilarDocuments retrieves documents with embeddings similar to the input vector
func GetSimilarDocuments(vector []float64, threshold float64) ([]Document, error) {
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	var embeddings []Embedding
	if err := db.Find(&embeddings).Error; err != nil {
		return nil, err
	}

	var similarDocs []Document
	for _, e := range embeddings {
		if err := e.AfterFind(nil); err != nil {
			continue
		}
		sim := cosineSimilarity(vector, e.Vector)
		if sim >= threshold {
			var doc Document
			if err := db.First(&doc, e.DocumentID).Error; err == nil {
				similarDocs = append(similarDocs, doc)
			}
		}
	}
	return similarDocs, nil
}

// cosineSimilarity calculates the cosine similarity between two vectors
func cosineSimilarity(v1, v2 []float64) float64 {
	if len(v1) != len(v2) {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(v1); i++ {
		dotProduct += v1[i] * v2[i]
		normA += v1[i] * v1[i]
		normB += v2[i] * v2[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// GetConnectedDocuments retrieves documents connected to the starting document via the graph
func GetConnectedDocuments(startDocID uint) ([]Document, error) {
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	visited := make(map[uint]bool)
	queue := []uint{startDocID}
	var connectedDocs []Document

	for len(queue) > 0 {
		currentDocID := queue[0]
		queue = queue[1:]

		if visited[currentDocID] {
			continue
		}
		visited[currentDocID] = true

		var doc Document
		if err := db.First(&doc, currentDocID).Error; err != nil {
			continue
		}
		connectedDocs = append(connectedDocs, doc)

		var currentNode GraphNode
		if err := db.First(&currentNode, "document_id = ?", currentDocID).Error; err != nil {
			continue
		}

		var edges []GraphEdge
		if err := db.Where("from_id = ?", currentNode.ID).Find(&edges).Error; err != nil {
			continue
		}

		for _, edge := range edges {
			var neighborNode GraphNode
			if err := db.First(&neighborNode, edge.ToID).Error; err != nil {
				continue
			}
			if !visited[neighborNode.DocumentID] {
				queue = append(queue, neighborNode.DocumentID)
			}
		}
	}
	return connectedDocs, nil
}

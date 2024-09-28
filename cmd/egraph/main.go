package main

import "fmt"

// Node represents a node in the graph database.
type Node struct {
	ID   int
	Type string
	Data map[string]interface{}
}

// Relationship represents a relationship between two nodes.
type Relationship struct {
	From int
	To   int
	Type string
	Data map[string]interface{}
}

// Graph represents the graph database.
type Graph struct {
	Nodes         []*Node
	Relationships []*Relationship
}

// CreateNode creates a new node in the graph.
func (g *Graph) CreateNode(nodeType string, data map[string]interface{}) *Node {
	node := &Node{
		ID:   len(g.Nodes),
		Type: nodeType,
		Data: data,
	}
	g.Nodes = append(g.Nodes, node)
	return node
}

// CreateRelationship creates a new relationship between two nodes.
func (g *Graph) CreateRelationship(from, to int, relationshipType string, data map[string]interface{}) *Relationship {
	relationship := &Relationship{
		From: from,
		To:   to,
		Type: relationshipType,
		Data: data,
	}
	g.Relationships = append(g.Relationships, relationship)
	return relationship
}

// Query executes a simple query on the graph.
func (g *Graph) Query(query string) ([]*Node, error) {
	// Implement query execution logic here.
	// This is a simplified example and does not support full Cypher query language.
	var results []*Node
	for _, node := range g.Nodes {
		if node.Type == "Person" {
			results = append(results, node)
		}
	}
	return results, nil
}

func main() {
	// Create a new graph database
	graph := &Graph{}

	// Create nodes
	alice := graph.CreateNode("Person", map[string]interface{}{"name": "Alice"})
	matrix := graph.CreateNode("Movie", map[string]interface{}{"title": "The Matrix"})

	// Create a relationship
	graph.CreateRelationship(alice.ID, matrix.ID, "WATCHED", nil)

	// Query the database
	results, err := graph.Query("MATCH (p:Person) RETURN p")
	if err != nil {
		panic(err)
	}

	for _, node := range results {
		fmt.Printf("%s\n", node.Data["name"])
	}
}

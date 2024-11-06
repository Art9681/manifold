// main.go

package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	MAX_CAPACITY = 256
	MIN_CAPACITY = 64
)

type Edge struct {
	targetVertexId int
	version        int64 // Timestamp for MVCC
	isDeleted      bool  // Flag to indicate logical deletion
}

type EdgeBlock struct {
	edges []Edge     // A sorted list of edges
	next  *EdgeBlock // Pointer to the next block in the skip list
	mu    sync.RWMutex
}

func (eb *EdgeBlock) isFull() bool {
	return len(eb.edges) >= MAX_CAPACITY
}

func (eb *EdgeBlock) insertEdge(edge Edge) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Insert edge into edges maintaining sorted order
	idx := sort.Search(len(eb.edges), func(i int) bool {
		return eb.edges[i].targetVertexId >= edge.targetVertexId
	})

	if idx < len(eb.edges) && eb.edges[idx].targetVertexId == edge.targetVertexId {
		// Edge already exists, update it
		eb.edges[idx] = edge
	} else {
		eb.edges = append(eb.edges, Edge{})    // make space
		copy(eb.edges[idx+1:], eb.edges[idx:]) // shift
		eb.edges[idx] = edge                   // insert
	}

	if eb.isFull() {
		eb.split()
	}
}

func (eb *EdgeBlock) split() {
	midIndex := len(eb.edges) / 2
	newBlock := &EdgeBlock{
		edges: append([]Edge(nil), eb.edges[midIndex:]...),
		next:  eb.next,
	}
	eb.edges = eb.edges[:midIndex]
	eb.next = newBlock
}

func (eb *EdgeBlock) checkUnderutilization() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if len(eb.edges) < MIN_CAPACITY {
		// Try to merge with next block
		if eb.next != nil && len(eb.edges)+len(eb.next.edges) <= MAX_CAPACITY {
			eb.edges = append(eb.edges, eb.next.edges...)
			eb.next = eb.next.next
		}
	}
}

type UnrolledSkipList struct {
	head          *EdgeBlock
	levelPointers []*EdgeBlock // Pointers for skip list levels
	mu            sync.RWMutex
}

func NewUnrolledSkipList() *UnrolledSkipList {
	return &UnrolledSkipList{
		head:          nil,
		levelPointers: nil,
	}
}

func (usl *UnrolledSkipList) search(targetVertexId int) (*Edge, *EdgeBlock) {
	usl.mu.RLock()
	defer usl.mu.RUnlock()

	currentBlock := usl.head
	for currentBlock != nil {
		currentBlock.mu.RLock()
		if len(currentBlock.edges) > 0 && targetVertexId <= currentBlock.edges[len(currentBlock.edges)-1].targetVertexId {
			idx := sort.Search(len(currentBlock.edges), func(i int) bool {
				return currentBlock.edges[i].targetVertexId >= targetVertexId
			})
			if idx < len(currentBlock.edges) && currentBlock.edges[idx].targetVertexId == targetVertexId {
				edge := &currentBlock.edges[idx]
				currentBlock.mu.RUnlock()
				return edge, currentBlock
			}
			currentBlock.mu.RUnlock()
			return nil, currentBlock
		}
		currentBlock.mu.RUnlock()
		currentBlock = currentBlock.next
	}
	return nil, nil
}

func (usl *UnrolledSkipList) insert(edge Edge) {
	usl.mu.Lock()
	defer usl.mu.Unlock()

	if usl.head == nil {
		usl.head = &EdgeBlock{
			edges: []Edge{edge},
			next:  nil,
		}
		return
	}

	currentBlock := usl.head
	for currentBlock != nil {
		currentBlock.mu.Lock()
		if len(currentBlock.edges) == 0 || edge.targetVertexId <= currentBlock.edges[len(currentBlock.edges)-1].targetVertexId {
			currentBlock.insertEdge(edge)
			currentBlock.mu.Unlock()
			return
		}
		nextBlock := currentBlock.next
		currentBlock.mu.Unlock()
		currentBlock = nextBlock
	}

	// Append to the last block
	newBlock := &EdgeBlock{
		edges: []Edge{edge},
		next:  nil,
	}
	currentBlock.next = newBlock
}

func (usl *UnrolledSkipList) delete(targetVertexId int, version int64) {
	usl.mu.Lock()
	defer usl.mu.Unlock()

	currentBlock := usl.head
	for currentBlock != nil {
		currentBlock.mu.Lock()
		if len(currentBlock.edges) > 0 && targetVertexId <= currentBlock.edges[len(currentBlock.edges)-1].targetVertexId {
			idx := sort.Search(len(currentBlock.edges), func(i int) bool {
				return currentBlock.edges[i].targetVertexId >= targetVertexId
			})
			if idx < len(currentBlock.edges) && currentBlock.edges[idx].targetVertexId == targetVertexId {
				currentBlock.edges[idx].isDeleted = true
				currentBlock.edges[idx].version = version
				currentBlock.checkUnderutilization()
			}
			currentBlock.mu.Unlock()
			return
		}
		nextBlock := currentBlock.next
		currentBlock.mu.Unlock()
		currentBlock = nextBlock
	}
}

func (usl *UnrolledSkipList) getActiveEdges() []int {
	usl.mu.RLock()
	defer usl.mu.RUnlock()

	var activeEdges []int
	currentBlock := usl.head
	currentTime := time.Now().UnixNano()
	for currentBlock != nil {
		currentBlock.mu.RLock()
		for _, edge := range currentBlock.edges {
			if edge.version <= currentTime && !edge.isDeleted {
				activeEdges = append(activeEdges, edge.targetVertexId)
			}
		}
		nextBlock := currentBlock.next
		currentBlock.mu.RUnlock()
		currentBlock = nextBlock
	}
	return activeEdges
}

type Vertex struct {
	id            int
	adjacencyList *UnrolledSkipList
	mu            sync.RWMutex
}

type VertexIdManager struct {
	externalToInternal map[int]int
	internalToExternal []int
	mu                 sync.Mutex
}

func NewVertexIdManager() *VertexIdManager {
	return &VertexIdManager{
		externalToInternal: make(map[int]int),
		internalToExternal: make([]int, 0),
	}
}

func (vm *VertexIdManager) getInternalId(externalId int) int {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if internalId, exists := vm.externalToInternal[externalId]; exists {
		return internalId
	}
	internalId := len(vm.internalToExternal)
	vm.externalToInternal[externalId] = internalId
	vm.internalToExternal = append(vm.internalToExternal, externalId)
	return internalId
}

func (vm *VertexIdManager) getExternalId(internalId int) int {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	return vm.internalToExternal[internalId]
}

type SortledtonGraph struct {
	vertices        map[int]*Vertex
	readWriteMu     sync.RWMutex // For concurrency control
	vertexIdManager *VertexIdManager
}

func NewSortledtonGraph() *SortledtonGraph {
	return &SortledtonGraph{
		vertices:        make(map[int]*Vertex),
		vertexIdManager: NewVertexIdManager(),
	}
}

func (g *SortledtonGraph) insertEdge(sourceId, targetId int) {
	internalSourceId := g.vertexIdManager.getInternalId(sourceId)
	internalTargetId := g.vertexIdManager.getInternalId(targetId)

	g.readWriteMu.RLock()
	vertex, exists := g.vertices[internalSourceId]
	if !exists {
		g.readWriteMu.RUnlock()
		g.readWriteMu.Lock()
		// Double-checking pattern
		if vertex, exists = g.vertices[internalSourceId]; !exists {
			vertex = &Vertex{
				id:            internalSourceId,
				adjacencyList: NewUnrolledSkipList(),
			}
			g.vertices[internalSourceId] = vertex
		}
		g.readWriteMu.Unlock()
	} else {
		g.readWriteMu.RUnlock()
	}

	// Lock the vertex
	vertex.mu.Lock()
	defer vertex.mu.Unlock()

	currentVersion := time.Now().UnixNano()
	edge := Edge{
		targetVertexId: internalTargetId,
		version:        currentVersion,
		isDeleted:      false,
	}
	vertex.adjacencyList.insert(edge)
}

func (g *SortledtonGraph) deleteEdge(sourceId, targetId int) {
	internalSourceId := g.vertexIdManager.getInternalId(sourceId)
	internalTargetId := g.vertexIdManager.getInternalId(targetId)

	g.readWriteMu.RLock()
	vertex, exists := g.vertices[internalSourceId]
	g.readWriteMu.RUnlock()
	if !exists {
		return
	}

	// Lock the vertex
	vertex.mu.Lock()
	defer vertex.mu.Unlock()

	currentVersion := time.Now().UnixNano()
	vertex.adjacencyList.delete(internalTargetId, currentVersion)
}

func (g *SortledtonGraph) getNeighbors(vertexId int) []int {
	internalId := g.vertexIdManager.getInternalId(vertexId)

	g.readWriteMu.RLock()
	vertex, exists := g.vertices[internalId]
	g.readWriteMu.RUnlock()
	if !exists {
		return []int{}
	}

	vertex.mu.RLock()
	defer vertex.mu.RUnlock()

	internalNeighbors := vertex.adjacencyList.getActiveEdges()
	neighbors := make([]int, len(internalNeighbors))
	for i, internalNeighborId := range internalNeighbors {
		neighbors[i] = g.vertexIdManager.getExternalId(internalNeighborId)
	}
	return neighbors
}

func intersectSortedSlices(a, b []int) []int {
	var intersection []int
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			intersection = append(intersection, a[i])
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	return intersection
}

func (g *SortledtonGraph) intersectNeighbors(vertexId1, vertexId2 int) []int {
	internalId1 := g.vertexIdManager.getInternalId(vertexId1)
	internalId2 := g.vertexIdManager.getInternalId(vertexId2)

	// Acquire locks in a global order to prevent deadlocks
	var firstVertex, secondVertex *Vertex
	if internalId1 < internalId2 {
		firstVertex = g.getVertex(internalId1)
		secondVertex = g.getVertex(internalId2)
	} else {
		firstVertex = g.getVertex(internalId2)
		secondVertex = g.getVertex(internalId1)
	}

	if firstVertex == nil || secondVertex == nil {
		return []int{}
	}

	firstVertex.mu.RLock()
	defer firstVertex.mu.RUnlock()

	secondVertex.mu.RLock()
	defer secondVertex.mu.RUnlock()

	internalNeighbors1 := firstVertex.adjacencyList.getActiveEdges()
	internalNeighbors2 := secondVertex.adjacencyList.getActiveEdges()

	return intersectInternalIdsToExternal(g.vertexIdManager, internalNeighbors1, internalNeighbors2)
}

func (g *SortledtonGraph) getVertex(internalId int) *Vertex {
	g.readWriteMu.RLock()
	vertex, exists := g.vertices[internalId]
	g.readWriteMu.RUnlock()
	if !exists {
		return nil
	}
	return vertex
}

func intersectInternalIdsToExternal(vm *VertexIdManager, a, b []int) []int {
	var intersection []int
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			externalId := vm.getExternalId(a[i])
			intersection = append(intersection, externalId)
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	return intersection
}

func (g *SortledtonGraph) garbageCollect() {
	g.readWriteMu.Lock()
	defer g.readWriteMu.Unlock()

	oldestActiveTransactionTimestamp := int64(0) // Placeholder

	for _, vertex := range g.vertices {
		vertex.mu.Lock()
		adjacencyList := vertex.adjacencyList
		currentBlock := adjacencyList.head
		var prevBlock *EdgeBlock = nil
		for currentBlock != nil {
			currentBlock.mu.Lock()
			newEdges := currentBlock.edges[:0]
			for _, edge := range currentBlock.edges {
				if edge.version < oldestActiveTransactionTimestamp && edge.isDeleted {
					continue
				}
				newEdges = append(newEdges, edge)
			}
			currentBlock.edges = newEdges
			if len(currentBlock.edges) == 0 {
				if prevBlock != nil {
					prevBlock.next = currentBlock.next
				} else {
					adjacencyList.head = currentBlock.next
				}
				nextBlock := currentBlock.next
				currentBlock.mu.Unlock()
				currentBlock = nextBlock
				continue
			}
			prevBlock = currentBlock
			nextBlock := currentBlock.next
			currentBlock.mu.Unlock()
			currentBlock = nextBlock
		}
		vertex.mu.Unlock()
	}
}

func (g *SortledtonGraph) StartGarbageCollector(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			g.garbageCollect()
		}
	}()
}

func main() {
	graph := NewSortledtonGraph()

	// Insert edges
	graph.insertEdge(1, 2)
	graph.insertEdge(1, 3)
	graph.insertEdge(2, 3)

	// Delete edge
	graph.deleteEdge(1, 2)

	// Retrieve neighbors
	neighborsOf1 := graph.getNeighbors(1)
	fmt.Println("Neighbors of 1:", neighborsOf1) // Should return [3]

	// Intersect neighbors
	commonNeighbors := graph.intersectNeighbors(1, 2)
	fmt.Println("Common neighbors of 1 and 2:", commonNeighbors) // Should return [3]
}

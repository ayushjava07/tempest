package dag

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrCycleDetected = errors.New("cycle detected in graph")
	ErrNodeNotFound  = errors.New("node not found in graph")
	ErrDuplicateNode = errors.New("node already exists in graph")
)

// Node represents a vertex in the DAG.
type Node struct {
	ID       string
	Metadata map[string]interface{}
}

// Graph represents a thread-safe directed acyclic graph.
type Graph struct {
	mu        sync.RWMutex
	nodes     map[string]*Node
	edges     map[string][]string // from -> []to (downstream)
	inDegrees map[string]int      // node -> count of upstream dependencies
	upstreams map[string][]string // to -> []from (upstream)
}

// New creates a new empty DAG.
func New() *Graph {
	return &Graph{
		nodes:     make(map[string]*Node),
		edges:     make(map[string][]string),
		inDegrees: make(map[string]int),
		upstreams: make(map[string][]string),
	}
}

// AddNode adds a new node to the graph.
func (g *Graph) AddNode(id string, metadata map[string]interface{}) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[id]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateNode, id)
	}

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	g.nodes[id] = &Node{ID: id, Metadata: metadata}
	g.edges[id] = make([]string, 0)
	g.upstreams[id] = make([]string, 0)
	g.inDegrees[id] = 0
	return nil
}

// AddEdge adds a directed dependency edge from 'from' to 'to'.
// 'to' depends on 'from' (i.e. 'from' must execute before 'to').
func (g *Graph) AddEdge(from, to string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[from]; !exists {
		return fmt.Errorf("%w: %s", ErrNodeNotFound, from)
	}
	if _, exists := g.nodes[to]; !exists {
		return fmt.Errorf("%w: %s", ErrNodeNotFound, to)
	}
	if from == to {
		return ErrCycleDetected
	}

	for _, existing := range g.edges[from] {
		if existing == to {
			return nil
		}
	}

	g.edges[from] = append(g.edges[from], to)
	g.upstreams[to] = append(g.upstreams[to], from)
	g.inDegrees[to]++
	return nil
}

// NodeCount returns the total number of nodes in the graph.
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// HasNode checks if a node exists.
func (g *Graph) HasNode(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.nodes[id]
	return exists
}

// Upstreams returns all direct upstream node IDs for the given node.
func (g *Graph) Upstreams(id string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, exists := g.nodes[id]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, id)
	}
	res := make([]string, len(g.upstreams[id]))
	copy(res, g.upstreams[id])
	sort.Strings(res)
	return res, nil
}

// Downstreams returns all direct downstream node IDs for the given node.
func (g *Graph) Downstreams(id string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, exists := g.nodes[id]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, id)
	}
	res := make([]string, len(g.edges[id]))
	copy(res, g.edges[id])
	sort.Strings(res)
	return res, nil
}

// TopologicalSort performs Kahn's algorithm to return a valid execution order.
// If the graph contains any cycle, ErrCycleDetected is returned.
func (g *Graph) TopologicalSort() ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	inDegree := make(map[string]int, len(g.nodes))
	for id, deg := range g.inDegrees {
		inDegree[id] = deg
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	result := make([]string, 0, len(g.nodes))

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		result = append(result, curr)

		nextNodes := make([]string, 0)
		for _, next := range g.edges[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				nextNodes = append(nextNodes, next)
			}
		}
		sort.Strings(nextNodes)
		queue = append(queue, nextNodes...)
	}

	if len(result) != len(g.nodes) {
		return nil, ErrCycleDetected
	}

	return result, nil
}

// Validate checks that the graph has no cycles and contains at least one root node.
func (g *Graph) Validate() error {
	_, err := g.TopologicalSort()
	return err
}

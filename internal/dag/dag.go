package dag

import (
	"fmt"
	"sync"
)

type Edge struct {
	From string
	To   string
}

type Graph struct {
	mu       sync.RWMutex
	nodes    map[string]bool
	edges    map[string][]string
	reverse  map[string][]string
}

func New() *Graph {
	return &Graph{
		nodes:   make(map[string]bool),
		edges:   make(map[string][]string),
		reverse: make(map[string][]string),
	}
}

func (g *Graph) AddNode(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[name] = true
	if g.edges[name] == nil {
		g.edges[name] = nil
	}
	if g.reverse[name] == nil {
		g.reverse[name] = nil
	}
}

func (g *Graph) RemoveNode(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.nodes, name)
	for _, from := range g.reverse[name] {
		g.removeEdge(from, name)
	}
	for _, to := range g.edges[name] {
		g.removeEdge(name, to)
	}
	delete(g.edges, name)
	delete(g.reverse, name)
}

func (g *Graph) removeEdge(from, to string) {
	edges := g.edges[from]
	for i, e := range edges {
		if e == to {
			g.edges[from] = append(edges[:i], edges[i+1:]...)
			break
		}
	}
	rev := g.reverse[to]
	for i, e := range rev {
		if e == from {
			g.reverse[to] = append(rev[:i], rev[i+1:]...)
			break
		}
	}
}

func (g *Graph) AddEdge(from, to string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.nodes[from] || !g.nodes[to] {
		return fmt.Errorf("node not found")
	}
	if g.hasPath(to, from) {
		return fmt.Errorf("would create cycle")
	}
	g.edges[from] = append(g.edges[from], to)
	g.reverse[to] = append(g.reverse[to], from)
	return nil
}

func (g *Graph) hasPath(from, to string) bool {
	visited := make(map[string]bool)
	stack := []string{from}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == to {
			return true
		}
		if visited[current] {
			continue
		}
		visited[current] = true
		for _, next := range g.edges[current] {
			stack = append(stack, next)
		}
	}
	return false
}

func (g *Graph) RemoveEdge(from, to string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.removeEdge(from, to)
}

func (g *Graph) HasNode(name string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[name]
}

func (g *Graph) Nodes() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make([]string, 0, len(g.nodes))
	for n := range g.nodes {
		result = append(result, n)
	}
	return result
}

func (g *Graph) TopologicalSort() ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	inDegree := make(map[string]int)
	for n := range g.nodes {
		inDegree[n] = 0
	}
	for _, toList := range g.edges {
		for _, to := range toList {
			inDegree[to]++
		}
	}
	var queue []string
	for n, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, n)
		}
	}
	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)
		for _, to := range g.edges[node] {
			inDegree[to]--
			if inDegree[to] == 0 {
				queue = append(queue, to)
			}
		}
	}
	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("cycle detected")
	}
	return result, nil
}

func (g *Graph) Dependents(node string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]string{}, g.edges[node]...)
}

func (g *Graph) Dependencies(node string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]string{}, g.reverse[node]...)
}
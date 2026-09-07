package graphmutator

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrNodeNotFound      = errors.New("graphmutator: node not found")
	ErrNodeAlreadyExists = errors.New("graphmutator: node already exists")
	ErrCycleDetected     = errors.New("graphmutator: cycle detected in graph")
	ErrDanglingEdge      = errors.New("graphmutator: dangling dependency edge")
	ErrEmptySubDAG       = errors.New("graphmutator: cannot inline empty sub-dag")
)

// NodeType identifies the execution semantics of a DAG node.
type NodeType string

const (
	NodeTask     NodeType = "TASK"
	NodeApproval NodeType = "APPROVAL"
	NodeFanOut   NodeType = "FAN_OUT"
	NodeFanIn    NodeType = "FAN_IN"
	NodeSubDAG   NodeType = "SUB_DAG"
)

// Node represents a step or checkpoint in a workflow DAG.
type Node struct {
	ID           string            `json:"id"`
	Type         NodeType          `json:"type"`
	Metadata     map[string]string `json:"metadata"`
	Dependencies []string          `json:"dependencies"` // upstream node IDs
}

// Clone returns a deep copy of the node.
func (n *Node) Clone() *Node {
	cp := &Node{
		ID:           n.ID,
		Type:         n.Type,
		Dependencies: make([]string, len(n.Dependencies)),
		Metadata:     make(map[string]string, len(n.Metadata)),
	}
	copy(cp.Dependencies, n.Dependencies)
	for k, v := range n.Metadata {
		cp.Metadata[k] = v
	}
	return cp
}

// DAG represents a mutable directed acyclic workflow graph.
type DAG struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

// New creates an empty DAG.
func New() *DAG {
	return &DAG{
		nodes: make(map[string]*Node),
	}
}

// AddNode registers a node in the DAG.
func (d *DAG) AddNode(n *Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.nodes[n.ID]; exists {
		return fmt.Errorf("%w: %s", ErrNodeAlreadyExists, n.ID)
	}

	d.nodes[n.ID] = n.Clone()
	return nil
}

// GetNode retrieves a node copy by ID.
func (d *DAG) GetNode(id string) (*Node, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	n, ok := d.nodes[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, id)
	}
	return n.Clone(), nil
}

// RemoveNode deletes a node and updates any dependent downstream references.
func (d *DAG) RemoveNode(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.nodes[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNodeNotFound, id)
	}

	delete(d.nodes, id)

	// Strip id from any other node's dependencies
	for _, n := range d.nodes {
		filtered := make([]string, 0, len(n.Dependencies))
		for _, dep := range n.Dependencies {
			if dep != id {
				filtered = append(filtered, dep)
			}
		}
		n.Dependencies = filtered
	}
	return nil
}

// AddEdge creates a directed dependency edge: `childID` depends on `parentID`.
func (d *DAG) AddEdge(parentID, childID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.nodes[parentID]; !ok {
		return fmt.Errorf("%w: parent %s", ErrNodeNotFound, parentID)
	}
	child, ok := d.nodes[childID]
	if !ok {
		return fmt.Errorf("%w: child %s", ErrNodeNotFound, childID)
	}

	for _, dep := range child.Dependencies {
		if dep == parentID {
			return nil // already exists
		}
	}
	child.Dependencies = append(child.Dependencies, parentID)
	return nil
}

// Clone creates an exact deep copy of the graph.
func (d *DAG) Clone() *DAG {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cp := New()
	for _, n := range d.nodes {
		cp.nodes[n.ID] = n.Clone()
	}
	return cp
}

// TopologicalSort returns node IDs in valid dependency execution order using Kahn's algorithm.
func (d *DAG) TopologicalSort() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	inDegree := make(map[string]int, len(d.nodes))
	dependents := make(map[string][]string, len(d.nodes)) // parent -> children

	for id, n := range d.nodes {
		inDegree[id] = 0
		_ = n
	}

	for id, n := range d.nodes {
		for _, dep := range n.Dependencies {
			if _, ok := d.nodes[dep]; !ok {
				return nil, fmt.Errorf("%w: %s -> %s", ErrDanglingEdge, id, dep)
			}
			dependents[dep] = append(dependents[dep], id)
			inDegree[id]++
		}
	}

	var zeroInDegree []string
	for id, deg := range inDegree {
		if deg == 0 {
			zeroInDegree = append(zeroInDegree, id)
		}
	}
	sort.Strings(zeroInDegree)

	var sorted []string
	for len(zeroInDegree) > 0 {
		curr := zeroInDegree[0]
		zeroInDegree = zeroInDegree[1:]
		sorted = append(sorted, curr)

		for _, child := range dependents[curr] {
			inDegree[child]--
			if inDegree[child] == 0 {
				zeroInDegree = append(zeroInDegree, child)
				sort.Strings(zeroInDegree)
			}
		}
	}

	if len(sorted) != len(d.nodes) {
		return nil, ErrCycleDetected
	}

	return sorted, nil
}

// Validate checks that all dependency edges are valid and no cycles exist.
func (d *DAG) Validate() error {
	_, err := d.TopologicalSort()
	return err
}

// InjectBefore inserts `newNode` directly before `targetID`.
// All nodes that previously fed into `targetID` now feed into `newNode`,
// and `targetID` depends solely on `newNode`.
func (d *DAG) InjectBefore(targetID string, newNode *Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	target, ok := d.nodes[targetID]
	if !ok {
		return fmt.Errorf("%w: target %s", ErrNodeNotFound, targetID)
	}
	if _, exists := d.nodes[newNode.ID]; exists {
		return fmt.Errorf("%w: %s", ErrNodeAlreadyExists, newNode.ID)
	}

	// Backup existing target dependencies
	origDeps := target.Dependencies

	// newNode inherits target's dependencies
	n := newNode.Clone()
	n.Dependencies = origDeps
	d.nodes[n.ID] = n

	// target now depends on newNode
	target.Dependencies = []string{n.ID}

	// Invariant check
	d.mu.Unlock()
	err := d.Validate()
	d.mu.Lock()
	if err != nil {
		// Rollback
		delete(d.nodes, n.ID)
		target.Dependencies = origDeps
		return err
	}

	return nil
}

// InjectAfter inserts `newNode` directly after `targetID`.
// All nodes that previously depended on `targetID` now depend on `newNode`,
// and `newNode` depends on `targetID`.
func (d *DAG) InjectAfter(targetID string, newNode *Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.nodes[targetID]; !ok {
		return fmt.Errorf("%w: target %s", ErrNodeNotFound, targetID)
	}
	if _, exists := d.nodes[newNode.ID]; exists {
		return fmt.Errorf("%w: %s", ErrNodeAlreadyExists, newNode.ID)
	}

	n := newNode.Clone()
	n.Dependencies = []string{targetID}
	d.nodes[n.ID] = n

	// Downstream nodes depending on targetID now depend on newNode
	for id, other := range d.nodes {
		if id == n.ID || id == targetID {
			continue
		}
		for i, dep := range other.Dependencies {
			if dep == targetID {
				other.Dependencies[i] = n.ID
			}
		}
	}

	d.mu.Unlock()
	err := d.Validate()
	d.mu.Lock()
	if err != nil {
		// Rollback
		delete(d.nodes, n.ID)
		for _, other := range d.nodes {
			for i, dep := range other.Dependencies {
				if dep == n.ID {
					other.Dependencies[i] = targetID
				}
			}
		}
		return err
	}

	return nil
}

// ExpandFanOut dynamically replaces a template node with N parallel branches (Map) and a Join node (Reduce).
func (d *DAG) ExpandFanOut(
	templateID string,
	count int,
	branchFactory func(index int) *Node,
	reduceNode *Node,
) error {
	if count <= 0 {
		return errors.New("graphmutator: branch count must be greater than zero")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	template, ok := d.nodes[templateID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNodeNotFound, templateID)
	}

	upstream := make([]string, len(template.Dependencies))
	copy(upstream, template.Dependencies)

	// Prepare parallel branch nodes
	var branchIDs []string
	branchNodes := make([]*Node, count)
	for i := 0; i < count; i++ {
		b := branchFactory(i)
		b.Dependencies = make([]string, len(upstream))
		copy(b.Dependencies, upstream)
		branchNodes[i] = b
		branchIDs = append(branchIDs, b.ID)
	}

	// Prepare reduce node
	red := reduceNode.Clone()
	red.Dependencies = branchIDs

	// Insert all branch nodes
	for _, b := range branchNodes {
		d.nodes[b.ID] = b
	}
	d.nodes[red.ID] = red

	// Rewire downstream nodes that depended on templateID to depend on reduceNode.ID
	for _, other := range d.nodes {
		if other.ID == red.ID {
			continue
		}
		for i, dep := range other.Dependencies {
			if dep == templateID {
				other.Dependencies[i] = red.ID
			}
		}
	}

	// Delete the placeholder template
	delete(d.nodes, templateID)

	d.mu.Unlock()
	err := d.Validate()
	d.mu.Lock()
	if err != nil {
		// Rollback
		delete(d.nodes, red.ID)
		for _, b := range branchNodes {
			delete(d.nodes, b.ID)
		}
		d.nodes[templateID] = template
		return err
	}

	return nil
}

// InlineSubDAG expands a composite node into an entire sub-DAG.
// Upstream nodes of compositeID become dependencies of sub-DAG's root nodes.
// Downstream nodes that depended on compositeID now depend on sub-DAG's exit nodes.
func (d *DAG) InlineSubDAG(compositeID string, sub *DAG) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	composite, ok := d.nodes[compositeID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNodeNotFound, compositeID)
	}

	sub.mu.RLock()
	if len(sub.nodes) == 0 {
		sub.mu.RUnlock()
		return ErrEmptySubDAG
	}

	// Identify sub-DAG roots (in-degree 0) and exit nodes (out-degree 0)
	subInDegree := make(map[string]int)
	subHasOutgoing := make(map[string]bool)

	for id := range sub.nodes {
		subInDegree[id] = 0
	}
	for id, n := range sub.nodes {
		for _, dep := range n.Dependencies {
			subInDegree[id]++
			subHasOutgoing[dep] = true
		}
	}

	var subRoots []string
	var subExits []string
	for id := range sub.nodes {
		if subInDegree[id] == 0 {
			subRoots = append(subRoots, id)
		}
		if !subHasOutgoing[id] {
			subExits = append(subExits, id)
		}
	}

	upstream := composite.Dependencies

	// Copy sub nodes into parent DAG
	for _, n := range sub.nodes {
		cloned := n.Clone()
		d.nodes[cloned.ID] = cloned
	}
	sub.mu.RUnlock()

	// Connect composite's upstream to sub roots
	for _, rootID := range subRoots {
		rNode := d.nodes[rootID]
		rNode.Dependencies = append(rNode.Dependencies, upstream...)
	}

	// Connect sub exits to downstream nodes that depended on compositeID
	for _, other := range d.nodes {
		if other.ID == compositeID {
			continue
		}
		var newDeps []string
		for _, dep := range other.Dependencies {
			if dep == compositeID {
				newDeps = append(newDeps, subExits...)
			} else {
				newDeps = append(newDeps, dep)
			}
		}
		other.Dependencies = newDeps
	}

	delete(d.nodes, compositeID)

	d.mu.Unlock()
	err := d.Validate()
	d.mu.Lock()
	if err != nil {
		// Restore composite
		d.nodes[compositeID] = composite
		return err
	}

	return nil
}

// GraphDiff captures differences between two DAG versions.
type GraphDiff struct {
	AddedNodes   []string
	RemovedNodes []string
	AddedEdges   [][2]string // [parent, child]
	RemovedEdges [][2]string
}

// Diff compares this DAG to another.
func (d *DAG) Diff(other *DAG) GraphDiff {
	d.mu.RLock()
	defer d.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	var diff GraphDiff

	// Added/Removed nodes
	for id := range other.nodes {
		if _, ok := d.nodes[id]; !ok {
			diff.AddedNodes = append(diff.AddedNodes, id)
		}
	}
	for id := range d.nodes {
		if _, ok := other.nodes[id]; !ok {
			diff.RemovedNodes = append(diff.RemovedNodes, id)
		}
	}

	sort.Strings(diff.AddedNodes)
	sort.Strings(diff.RemovedNodes)
	return diff
}

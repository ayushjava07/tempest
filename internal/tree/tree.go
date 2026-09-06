package tree

import (
	"sort"
	"sync"
)

type Node[T any] struct {
	Value    T
	Children []*Node[T]
	Parent   *Node[T]
}

func NewNode[T any](value T) *Node[T] {
	return &Node[T]{Value: value}
}

func (n *Node[T]) Add(child *Node[T]) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

func (n *Node[T]) Remove(child *Node[T]) {
	for i, c := range n.Children {
		if c == child {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			child.Parent = nil
			return
		}
	}
}

func (n *Node[T]) Depth() int {
	depth := 0
	current := n
	for current.Parent != nil {
		depth++
		current = current.Parent
	}
	return depth
}

func (n *Node[T]) Root() *Node[T] {
	current := n
	for current.Parent != nil {
		current = current.Parent
	}
	return current
}

func (n *Node[T]) IsLeaf() bool {
	return len(n.Children) == 0
}

func (n *Node[T]) IsRoot() bool {
	return n.Parent == nil
}

func (n *Node[T]) Siblings() []*Node[T] {
	if n.Parent == nil {
		return nil
	}
	var siblings []*Node[T]
	for _, c := range n.Parent.Children {
		if c != n {
			siblings = append(siblings, c)
		}
	}
	return siblings
}

type Tree[T any] struct {
	mu   sync.RWMutex
	root *Node[T]
	size int
}

func NewTree[T any](root *Node[T]) *Tree[T] {
	return &Tree[T]{root: root, size: 1}
}

func (t *Tree[T]) Root() *Node[T] {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.root
}

func (t *Tree[T]) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

func (t *Tree[T]) Add(parent, child *Node[T]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent.Add(child)
	t.size++
}

func (t *Tree[T]) Remove(node *Node[T]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if node.Parent != nil {
		node.Parent.Remove(node)
		t.size--
	}
}

func (t *Tree[T]) BFS(fn func(*Node[T])) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.root == nil {
		return
	}
	queue := []*Node[T]{t.root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		fn(node)
		queue = append(queue, node.Children...)
	}
}

func (t *Tree[T]) DFS(fn func(*Node[T])) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.root == nil {
		return
	}
	stack := []*Node[T]{t.root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fn(node)
		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, node.Children[i])
		}
	}
}

func (t *Tree[T]) Leaves() []*Node[T] {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var leaves []*Node[T]
	t.walkLeaves(t.root, &leaves)
	return leaves
}

func (t *Tree[T]) walkLeaves(node *Node[T], leaves *[]*Node[T]) {
	if node == nil {
		return
	}
	if node.IsLeaf() {
		*leaves = append(*leaves, node)
	}
	for _, c := range node.Children {
		t.walkLeaves(c, leaves)
	}
}

func (t *Tree[T]) Depth() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.maxDepth(t.root)
}

func (t *Tree[T]) maxDepth(node *Node[T]) int {
	if node == nil {
		return 0
	}
	max := 0
	for _, c := range node.Children {
		d := t.maxDepth(c)
		if d > max {
			max = d
		}
	}
	return max + 1
}

func (t *Tree[T]) Sort(less func(a, b *Node[T]) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sort.Slice(t.root.Children, func(i, j int) bool {
		return less(t.root.Children[i], t.root.Children[j])
	})
}

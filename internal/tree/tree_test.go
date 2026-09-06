package tree

import (
	"sort"
	"testing"
)

func TestTree_Basic(t *testing.T) {
	root := NewNode(1)
	tree := NewTree(root)
	if tree.Size() != 1 {
		t.Errorf("expected 1, got %d", tree.Size())
	}
}

func TestTree_Add(t *testing.T) {
	root := NewNode(1)
	child := NewNode(2)
	tree := NewTree(root)
	tree.Add(root, child)
	if tree.Size() != 2 {
		t.Errorf("expected 2, got %d", tree.Size())
	}
	if child.Parent != root {
		t.Error("expected parent")
	}
}

func TestTree_Remove(t *testing.T) {
	root := NewNode(1)
	child := NewNode(2)
	tree := NewTree(root)
	tree.Add(root, child)
	tree.Remove(child)
	if tree.Size() != 1 {
		t.Errorf("expected 1, got %d", tree.Size())
	}
}

func TestTree_Depth(t *testing.T) {
	root := NewNode(1)
	c1 := NewNode(2)
	c2 := NewNode(3)
	c3 := NewNode(4)
	tree := NewTree(root)
	tree.Add(root, c1)
	tree.Add(root, c2)
	tree.Add(c1, c3)
	if tree.Depth() != 3 {
		t.Errorf("expected 3, got %d", tree.Depth())
	}
}

func TestTree_BFS(t *testing.T) {
	root := NewNode(1)
	c1 := NewNode(2)
	c2 := NewNode(3)
	tree := NewTree(root)
	tree.Add(root, c1)
	tree.Add(root, c2)
	var order []int
	tree.BFS(func(n *Node[int]) {
		order = append(order, n.Value)
	})
	if len(order) != 3 || order[0] != 1 {
		t.Error("wrong BFS order")
	}
}

func TestTree_DFS(t *testing.T) {
	root := NewNode(1)
	c1 := NewNode(2)
	c2 := NewNode(3)
	tree := NewTree(root)
	tree.Add(root, c1)
	tree.Add(root, c2)
	var order []int
	tree.DFS(func(n *Node[int]) {
		order = append(order, n.Value)
	})
	if len(order) != 3 || order[0] != 1 {
		t.Error("wrong DFS order")
	}
}

func TestTree_Leaves(t *testing.T) {
	root := NewNode(1)
	c1 := NewNode(2)
	c2 := NewNode(3)
	tree := NewTree(root)
	tree.Add(root, c1)
	tree.Add(root, c2)
	tree.Add(c1, NewNode(4))
	leaves := tree.Leaves()
	if len(leaves) != 2 {
		t.Errorf("expected 2 leaves, got %d", len(leaves))
	}
}

func TestNode_Depth(t *testing.T) {
	root := NewNode(1)
	child := NewNode(2)
	grandchild := NewNode(3)
	root.Add(child)
	child.Add(grandchild)
	if grandchild.Depth() != 2 {
		t.Errorf("expected 2, got %d", grandchild.Depth())
	}
}

func TestNode_Root(t *testing.T) {
	root := NewNode(1)
	child := NewNode(2)
	root.Add(child)
	if child.Root() != root {
		t.Error("expected root")
	}
}

func TestNode_IsLeaf(t *testing.T) {
	node := NewNode(1)
	if !node.IsLeaf() {
		t.Error("expected leaf")
	}
	node.Add(NewNode(2))
	if node.IsLeaf() {
		t.Error("expected not leaf")
	}
}

func TestNode_IsRoot(t *testing.T) {
	root := NewNode(1)
	child := NewNode(2)
	root.Add(child)
	if !root.IsRoot() {
		t.Error("expected root")
	}
	if child.IsRoot() {
		t.Error("expected not root")
	}
}

func TestNode_Siblings(t *testing.T) {
	root := NewNode(1)
	c1 := NewNode(2)
	c2 := NewNode(3)
	root.Add(c1)
	root.Add(c2)
	sibs := c1.Siblings()
	if len(sibs) != 1 || sibs[0] != c2 {
		t.Error("expected c2 as sibling")
	}
}

func TestTree_Sort(t *testing.T) {
	root := NewNode(1)
	root.Add(NewNode(3))
	root.Add(NewNode(1))
	root.Add(NewNode(2))
	tree := NewTree(root)
	tree.Sort(func(a, b *Node[int]) bool {
		return a.Value < b.Value
	})
	values := make([]int, len(root.Children))
	for i, c := range root.Children {
		values[i] = c.Value
	}
	if !sort.IntsAreSorted(values) {
		t.Error("expected sorted")
	}
}

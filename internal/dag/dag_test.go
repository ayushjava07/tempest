package dag

import (
	"testing"
)

func TestDAG_LinearExecution(t *testing.T) {
	g := New()
	_ = g.AddNode("A", nil)
	_ = g.AddNode("B", nil)
	_ = g.AddNode("C", nil)

	if err := g.AddEdge("A", "B"); err != nil {
		t.Fatalf("unexpected error adding edge A->B: %v", err)
	}
	if err := g.AddEdge("B", "C"); err != nil {
		t.Fatalf("unexpected error adding edge B->C: %v", err)
	}

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error in sort: %v", err)
	}

	expected := []string{"A", "B", "C"}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("expected index %d to be %s, got %s", i, v, order[i])
		}
	}
}

func TestDAG_BranchAndJoin(t *testing.T) {
	g := New()
	_ = g.AddNode("start", nil)
	_ = g.AddNode("branch1", nil)
	_ = g.AddNode("branch2", nil)
	_ = g.AddNode("join", nil)

	_ = g.AddEdge("start", "branch1")
	_ = g.AddEdge("start", "branch2")
	_ = g.AddEdge("branch1", "join")
	_ = g.AddEdge("branch2", "join")

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if order[0] != "start" {
		t.Errorf("expected first node to be start, got %s", order[0])
	}
	if order[3] != "join" {
		t.Errorf("expected last node to be join, got %s", order[3])
	}
}

func TestDAG_CycleDetection(t *testing.T) {
	g := New()
	_ = g.AddNode("A", nil)
	_ = g.AddNode("B", nil)
	_ = g.AddNode("C", nil)

	_ = g.AddEdge("A", "B")
	_ = g.AddEdge("B", "C")
	_ = g.AddEdge("C", "A")

	_, err := g.TopologicalSort()
	if err != ErrCycleDetected {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
	if err := g.Validate(); err != ErrCycleDetected {
		t.Fatalf("expected ErrCycleDetected on Validate(), got %v", err)
	}
}

func TestDAG_SelfReferentialCycle(t *testing.T) {
	g := New()
	_ = g.AddNode("A", nil)

	err := g.AddEdge("A", "A")
	if err != ErrCycleDetected {
		t.Fatalf("expected ErrCycleDetected for self-loop, got %v", err)
	}
}

func TestDAG_DuplicateNode(t *testing.T) {
	g := New()
	if err := g.AddNode("A", nil); err != nil {
		t.Fatalf("failed to add node A: %v", err)
	}
	if err := g.AddNode("A", nil); err == nil {
		t.Fatalf("expected error on duplicate node, got nil")
	}
}

func TestDAG_NodeQueries(t *testing.T) {
	g := New()
	_ = g.AddNode("parent", nil)
	_ = g.AddNode("child1", nil)
	_ = g.AddNode("child2", nil)

	_ = g.AddEdge("parent", "child1")
	_ = g.AddEdge("parent", "child2")

	if g.NodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", g.NodeCount())
	}
	if !g.HasNode("parent") || g.HasNode("unknown") {
		t.Errorf("incorrect HasNode results")
	}

	downs, err := g.Downstreams("parent")
	if err != nil || len(downs) != 2 {
		t.Errorf("expected 2 downstreams, got %v (err: %v)", downs, err)
	}

	ups, err := g.Upstreams("child1")
	if err != nil || len(ups) != 1 || ups[0] != "parent" {
		t.Errorf("expected 1 upstream (parent), got %v (err: %v)", ups, err)
	}
}

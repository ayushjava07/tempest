package dag

import (
	"testing"
)

func TestGraph_AddNode(t *testing.T) {
	g := New()
	g.AddNode("a")
	if !g.HasNode("a") {
		t.Error("expected node a")
	}
	if g.HasNode("b") {
		t.Error("expected no node b")
	}
}

func TestGraph_AddEdge(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestGraph_AddEdge_Cycle(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddEdge("a", "b")
	if err := g.AddEdge("b", "a"); err == nil {
		t.Error("expected cycle error")
	}
}

func TestGraph_RemoveNode(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddEdge("a", "b")
	g.RemoveNode("a")
	if g.HasNode("a") {
		t.Error("expected a removed")
	}
	if !g.HasNode("b") {
		t.Error("expected b still there")
	}
}

func TestGraph_RemoveEdge(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddEdge("a", "b")
	g.RemoveEdge("a", "b")
}

func TestGraph_TopologicalSort(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddNode("c")
	g.AddNode("d")
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "d")
	g.AddEdge("c", "d")
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 4 {
		t.Errorf("expected 4, got %d", len(sorted))
	}
	idx := make(map[string]int)
	for i, n := range sorted {
		idx[n] = i
	}
	if idx["a"] >= idx["b"] || idx["a"] >= idx["c"] {
		t.Error("a before b and c")
	}
	if idx["b"] >= idx["d"] || idx["c"] >= idx["d"] {
		t.Error("b and c before d")
	}
}

func TestGraph_Dependents(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddEdge("a", "b")
	deps := g.Dependents("a")
	if len(deps) != 1 || deps[0] != "b" {
		t.Error("expected dependents b")
	}
}

func TestGraph_Dependencies(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddEdge("a", "b")
	deps := g.Dependencies("b")
	if len(deps) != 1 || deps[0] != "a" {
		t.Error("expected dependency a")
	}
}

func TestGraph_ComplexDAG(t *testing.T) {
	g := New()
	nodes := []string{"a", "b", "c", "d", "e", "f"}
	for _, n := range nodes {
		g.AddNode(n)
	}
	edges := [][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}, {"d", "e"}, {"d", "f"}}
	for _, e := range edges {
		if err := g.AddEdge(e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 6 {
		t.Errorf("expected 6, got %d", len(sorted))
	}
}
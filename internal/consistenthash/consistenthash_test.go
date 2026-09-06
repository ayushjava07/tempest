package consistenthash

import (
	"fmt"
	"testing"
)

func TestHashRing_Basic(t *testing.T) {
	hr := New(100)
	hr.Add("node1")
	hr.Add("node2")
	hr.Add("node3")
	if hr.Len() != 3 {
		t.Errorf("expected 3, got %d", hr.Len())
	}
}

func TestHashRing_Get(t *testing.T) {
	hr := New(100)
	hr.Add("node1")
	hr.Add("node2")
	result := hr.Get("some-key")
	if result == "" {
		t.Error("expected non-empty result")
	}
	if result != "node1" && result != "node2" {
		t.Errorf("unexpected node: %s", result)
	}
}

func TestHashRing_Consistent(t *testing.T) {
	hr := New(100)
	hr.Add("node1")
	hr.Add("node2")
	result1 := hr.Get("key-1")
	result2 := hr.Get("key-1")
	if result1 != result2 {
		t.Error("expected consistent result")
	}
}

func TestHashRing_Remove(t *testing.T) {
	hr := New(100)
	hr.Add("node1")
	hr.Add("node2")
	hr.Remove("node1")
	if hr.Len() != 1 {
		t.Errorf("expected 1, got %d", hr.Len())
	}
	result := hr.Get("any-key")
	if result != "node2" {
		t.Errorf("expected node2, got %s", result)
	}
}

func TestHashRing_RemoveNonexistent(t *testing.T) {
	hr := New(100)
	hr.Remove("nope")
	if hr.Len() != 0 {
		t.Error("expected 0")
	}
}

func TestHashRing_DuplicateAdd(t *testing.T) {
	hr := New(100)
	hr.Add("node1")
	hr.Add("node1")
	if hr.Len() != 1 {
		t.Errorf("expected 1, got %d", hr.Len())
	}
}

func TestHashRing_Nodes(t *testing.T) {
	hr := New(10)
	hr.Add("a")
	hr.Add("b")
	nodes := hr.Nodes()
	if len(nodes) != 2 {
		t.Errorf("expected 2, got %d", len(nodes))
	}
}

func TestHashRing_Empty(t *testing.T) {
	hr := New(100)
	if hr.Get("key") != "" {
		t.Error("expected empty for no nodes")
	}
}

func TestHashRing_Distribution(t *testing.T) {
	hr := New(150)
	hr.Add("node1")
	hr.Add("node2")
	hr.Add("node3")
	keys := make([]string, 3000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	dist := hr.Distribution(keys)
	total := 0
	for _, count := range dist {
		total += count
	}
	if total != 3000 {
		t.Errorf("expected 3000 total, got %d", total)
	}
}

func TestHashRing_LargeCluster(t *testing.T) {
	hr := New(100)
	for i := 0; i < 50; i++ {
		hr.Add(fmt.Sprintf("node-%d", i))
	}
	if hr.Len() != 50 {
		t.Errorf("expected 50, got %d", hr.Len())
	}
	for i := 0; i < 100; i++ {
		result := hr.Get(fmt.Sprintf("key-%d", i))
		if result == "" {
			t.Error("expected non-empty result")
		}
	}
}

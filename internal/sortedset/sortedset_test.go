package sortedset

import (
	"testing"
)

func TestSortedSet_Add(t *testing.T) {
	ss := New[string]()
	ss.Add("c", 3)
	ss.Add("a", 1)
	ss.Add("b", 2)
	if ss.Len() != 3 {
		t.Errorf("expected 3, got %d", ss.Len())
	}
}

func TestSortedSet_Sorted(t *testing.T) {
	ss := New[string]()
	ss.Add("c", 3)
	ss.Add("a", 1)
	ss.Add("b", 2)
	items := ss.Items()
	if items[0].Score != 1 || items[1].Score != 2 || items[2].Score != 3 {
		t.Error("expected sorted by score")
	}
}

func TestSortedSet_Remove(t *testing.T) {
	ss := New[int]()
	ss.Add(1, 10)
	ss.Add(2, 20)
	if !ss.Remove(0) {
		t.Error("expected success")
	}
	if ss.Len() != 1 {
		t.Errorf("expected 1, got %d", ss.Len())
	}
}

func TestSortedSet_RemoveInvalid(t *testing.T) {
	ss := New[int]()
	if ss.Remove(0) {
		t.Error("expected false")
	}
}

func TestSortedSet_Get(t *testing.T) {
	ss := New[string]()
	ss.Add("x", 5)
	entry, ok := ss.Get(0)
	if !ok || entry.Member != "x" {
		t.Error("expected x")
	}
}

func TestSortedSet_Range(t *testing.T) {
	ss := New[int]()
	for i := 0; i < 10; i++ {
		ss.Add(i, float64(i))
	}
	r := ss.Range(2, 5)
	if len(r) != 3 {
		t.Errorf("expected 3, got %d", len(r))
	}
}

func TestSortedSet_MinMax(t *testing.T) {
	ss := New[int]()
	ss.Add(5, 50)
	ss.Add(1, 10)
	ss.Add(9, 90)
	min, _ := ss.Min()
	max, _ := ss.Max()
	if min.Score != 10 || max.Score != 90 {
		t.Error("expected min=10 max=90")
	}
}

func TestSortedSet_Empty(t *testing.T) {
	ss := New[int]()
	if _, ok := ss.Min(); ok {
		t.Error("expected empty min")
	}
	if _, ok := ss.Max(); ok {
		t.Error("expected empty max")
	}
}

func TestSortedSet_Contains(t *testing.T) {
	ss := New[string]()
	ss.Add("hello", 1)
	if !ss.Contains("hello") {
		t.Error("expected contains")
	}
	if ss.Contains("world") {
		t.Error("expected not contains")
	}
}

func TestSortedSet_Index(t *testing.T) {
	ss := New[string]()
	ss.Add("a", 1)
	ss.Add("b", 2)
	if ss.Index("b") != 1 {
		t.Errorf("expected 1, got %d", ss.Index("b"))
	}
	if ss.Index("c") != -1 {
		t.Error("expected -1")
	}
}

func TestSortedSet_Clear(t *testing.T) {
	ss := New[int]()
	ss.Add(1, 10)
	ss.Clear()
	if ss.Len() != 0 {
		t.Error("expected empty")
	}
}

func TestSortedSet_Duplicates(t *testing.T) {
	ss := New[int]()
	ss.Add(1, 10)
	ss.Add(2, 10)
	if ss.Len() != 2 {
		t.Error("expected 2 entries with same score")
	}
}

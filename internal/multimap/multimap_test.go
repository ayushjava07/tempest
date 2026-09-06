package multimap

import (
	"sort"
	"sync"
	"testing"
)

func TestMultiMap_AddGet(t *testing.T) {
	m := New[string, int]()
	m.Add("a", 1)
	m.Add("a", 2)
	vals := m.Get("a")
	if len(vals) != 2 {
		t.Errorf("expected 2, got %d", len(vals))
	}
}

func TestMultiMap_GetMissing(t *testing.T) {
	m := New[string, int]()
	vals := m.Get("missing")
	if len(vals) != 0 {
		t.Error("expected empty")
	}
}

func TestMultiMap_Remove(t *testing.T) {
	m := New[string, int]()
	m.Add("a", 1)
	if !m.Remove("a") {
		t.Error("expected true")
	}
	if m.Contains("a") {
		t.Error("expected not contained")
	}
}

func TestMultiMap_RemoveMissing(t *testing.T) {
	m := New[string, int]()
	if m.Remove("nope") {
		t.Error("expected false")
	}
}

func TestMultiMap_Contains(t *testing.T) {
	m := New[string, int]()
	m.Add("x", 10)
	if !m.Contains("x") {
		t.Error("expected contains")
	}
}

func TestMultiMap_Len(t *testing.T) {
	m := New[string, int]()
	m.Add("a", 1)
	m.Add("a", 2)
	m.Add("b", 3)
	if m.Len() != 3 {
		t.Errorf("expected 3, got %d", m.Len())
	}
}

func TestMultiMap_Keys(t *testing.T) {
	m := New[string, int]()
	m.Add("c", 1)
	m.Add("a", 2)
	m.Add("b", 3)
	keys := m.Keys()
	sort.Strings(keys)
	if len(keys) != 3 {
		t.Errorf("expected 3, got %d", len(keys))
	}
}

func TestMultiMap_Clear(t *testing.T) {
	m := New[string, int]()
	m.Add("a", 1)
	m.Add("b", 2)
	m.Clear()
	if m.Len() != 0 {
		t.Error("expected empty")
	}
}

func TestMultiMap_ForEach(t *testing.T) {
	m := New[string, int]()
	m.Add("a", 1)
	m.Add("a", 2)
	var total int
	m.ForEach(func(k string, vals []int) {
		for _, v := range vals {
			total += v
		}
	})
	if total != 3 {
		t.Errorf("expected 3, got %d", total)
	}
}

func TestMultiMap_Concurrent(t *testing.T) {
	m := New[int, int]()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Add(n, n)
			m.Get(n)
			m.Contains(n)
			m.Len()
		}(i)
	}
	wg.Wait()
	if m.Len() != 100 {
		t.Errorf("expected 100, got %d", m.Len())
	}
}

func TestMultiMap_RemoveValue(t *testing.T) {
	m := New[string, int]()
	m.Add("a", 1)
	m.Add("a", 2)
	m.RemoveValue("a", 1)
	vals := m.Get("a")
	if len(vals) != 1 || vals[0] != 2 {
		t.Error("expected only value 2 remaining")
	}
}

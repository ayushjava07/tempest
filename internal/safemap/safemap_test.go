package safemap

import (
	"sync"
	"testing"
)

func TestMap_Basic(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	v, ok := m.Get("a")
	if !ok || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestMap_Delete(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	if !m.Delete("a") {
		t.Error("expected true")
	}
	if m.Delete("a") {
		t.Error("expected false")
	}
}

func TestMap_Has(t *testing.T) {
	m := New[string, int]()
	m.Set("x", 10)
	if !m.Has("x") {
		t.Error("expected has")
	}
	if m.Has("y") {
		t.Error("expected not has")
	}
}

func TestMap_Len(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	if m.Len() != 2 {
		t.Errorf("expected 2, got %d", m.Len())
	}
}

func TestMap_Keys_Values(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	if len(m.Keys()) != 2 {
		t.Error("expected 2 keys")
	}
	if len(m.Values()) != 2 {
		t.Error("expected 2 values")
	}
}

func TestMap_Items(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	items := m.Items()
	if len(items) != 1 {
		t.Error("expected 1 item")
	}
}

func TestMap_Clear(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	m.Clear()
	if m.Len() != 0 {
		t.Error("expected empty")
	}
}

func TestMap_Concurrent(t *testing.T) {
	m := New[int, int]()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Set(n, n)
			m.Get(n)
			m.Has(n)
			m.Len()
		}(i)
	}
	wg.Wait()
	if m.Len() != 100 {
		t.Errorf("expected 100, got %d", m.Len())
	}
}

func TestMap_GetOrSet(t *testing.T) {
	m := New[string, int]()
	v := m.GetOrSet("a", 1)
	if v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
	v = m.GetOrSet("a", 2)
	if v != 1 {
		t.Error("expected existing value 1")
	}
}

func TestMap_Update(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	if !m.Update("a", func(v int) int { return v * 2 }) {
		t.Error("expected true")
	}
	v, _ := m.Get("a")
	if v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
	if m.Update("missing", func(v int) int { return v }) {
		t.Error("expected false")
	}
}

func TestMap_ForEach(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	var sum int
	m.ForEach(func(k string, v int) { sum += v })
	if sum != 3 {
		t.Errorf("expected 3, got %d", sum)
	}
}

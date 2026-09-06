package lru

import (
	"sync"
	"testing"
)

func TestCache_PutGet(t *testing.T) {
	c := New[string, int](3)
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestCache_Eviction(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	if c.Len() != 2 {
		t.Errorf("expected 2, got %d", c.Len())
	}
	_, ok := c.Get("a")
	if ok {
		t.Error("expected a to be evicted")
	}
}

func TestCache_Update(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("a", 2)
	v, _ := c.Get("a")
	if v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
	if c.Len() != 1 {
		t.Errorf("expected 1, got %d", c.Len())
	}
}

func TestCache_LRU_Eviction(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")
	c.Put("c", 3)
	_, ok := c.Get("a")
	if !ok {
		t.Error("expected a to survive (was recently accessed)")
	}
	_, ok = c.Get("b")
	if ok {
		t.Error("expected b to be evicted")
	}
}

func TestCache_Remove(t *testing.T) {
	c := New[string, int](3)
	c.Put("a", 1)
	if !c.Remove("a") {
		t.Error("expected true")
	}
	if c.Remove("a") {
		t.Error("expected false")
	}
}

func TestCache_Keys(t *testing.T) {
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	keys := c.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2, got %d", len(keys))
	}
}

func TestCache_Clear(t *testing.T) {
	c := New[string, int](3)
	c.Put("a", 1)
	c.Clear()
	if c.Len() != 0 {
		t.Error("expected empty")
	}
}

func TestCache_OnEvict(t *testing.T) {
	c := New[string, int](2)
	var evicted []string
	c.OnEvict(func(k string, v int) {
		evicted = append(evicted, k)
	})
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	if len(evicted) != 1 || evicted[0] != "a" {
		t.Error("expected a evicted")
	}
}

func TestCache_Miss(t *testing.T) {
	c := New[string, int](3)
	_, ok := c.Get("missing")
	if ok {
		t.Error("expected miss")
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := New[int, int](100)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Put(n, n)
			c.Get(n)
		}(i)
	}
	wg.Wait()
}

package pool

import (
	"sync"
	"testing"
)

func TestPool_GetPut(t *testing.T) {
	p := New(10, func() int { return 42 }, func(i int) {})
	v := p.Get()
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	p.Put(v)
	if p.Size() != 1 {
		t.Errorf("expected 1, got %d", p.Size())
	}
}

func TestPool_Reuse(t *testing.T) {
	p := New(10, func() int { return 0 }, func(i int) {})
	p.Put(99)
	v := p.Get()
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

func TestPool_OverCapacity(t *testing.T) {
	p := New(2, func() int { return 0 }, func(i int) {})
	p.Put(1)
	p.Put(2)
	p.Put(3)
	if p.Size() != 2 {
		t.Errorf("expected 2, got %d", p.Size())
	}
}

func TestPool_Clear(t *testing.T) {
	p := New(10, func() int { return 0 }, func(i int) {})
	p.Put(1)
	p.Put(2)
	p.Clear()
	if p.Size() != 0 {
		t.Error("expected empty pool")
	}
}

func TestPool_Concurrent(t *testing.T) {
	p := New(100, func() int { return 0 }, func(i int) {})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			p.Put(n)
			_ = p.Get()
		}(i)
	}
	wg.Wait()
}

func TestPool_Cap(t *testing.T) {
	p := New(50, func() int { return 0 }, func(i int) {})
	if p.Cap() != 50 {
		t.Errorf("expected 50, got %d", p.Cap())
	}
}

func TestPool_Reset(t *testing.T) {
	reset := false
	p := New(10, func() string { return "" }, func(s string) { reset = true })
	p.Put("hello")
	if !reset {
		t.Error("expected reset to be called")
	}
}

func TestPool_NewItems(t *testing.T) {
	count := 0
	p := New(5, func() int { count++; return count }, func(i int) {})
	a := p.Get()
	b := p.Get()
	if a == b {
		t.Error("expected different items from factory")
	}
	if count != 2 {
		t.Errorf("expected 2 factory calls, got %d", count)
	}
}

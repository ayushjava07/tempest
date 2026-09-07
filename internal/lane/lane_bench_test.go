package lane

import (
	"sync"
	"testing"
)

func TestAtomicQuantumCounter(t *testing.T) {
	c := NewAtomicQuantumCounter(10)
	if c.Load() != 10 {
		t.Fatalf("expected 10, got %d", c.Load())
	}

	if !c.TryConsume(6) {
		t.Fatalf("expected consume 6 to succeed")
	}
	if c.Load() != 4 {
		t.Fatalf("expected 4 remaining, got %d", c.Load())
	}

	// Cannot consume 5
	if c.TryConsume(5) {
		t.Fatalf("consume 5 should have failed")
	}

	c.Add(10)
	if c.Load() != 14 {
		t.Fatalf("expected 14, got %d", c.Load())
	}

	c.Reset()
	if c.Load() != 0 {
		t.Fatalf("expected 0, got %d", c.Load())
	}
}

func BenchmarkAtomicQuantumCounterContention(b *testing.B) {
	counter := NewAtomicQuantumCounter(1000000)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Add(1)
			_ = counter.TryConsume(1)
		}
	})
}

func BenchmarkMutexCounterContention(b *testing.B) {
	var mu sync.Mutex
	val := int64(1000000)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			val++
			if val >= 1 {
				val--
			}
			mu.Unlock()
		}
	})
}

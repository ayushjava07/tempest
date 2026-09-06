package semaphore

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSemaphore_Basic(t *testing.T) {
	s := New(3)
	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if s.Available() != 2 {
		t.Errorf("expected 2, got %d", s.Available())
	}
	s.Release()
	if s.Available() != 3 {
		t.Errorf("expected 3, got %d", s.Available())
	}
}

func TestSemaphore_Overflow(t *testing.T) {
	s := New(2)
	s.Acquire(context.Background())
	s.Acquire(context.Background())
	if s.Available() != 0 {
		t.Error("expected 0 available")
	}
}

func TestSemaphore_ContextCancel(t *testing.T) {
	s := New(1)
	s.Acquire(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.Acquire(ctx); err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	s.Release()
}

func TestSemaphore_TryAcquire(t *testing.T) {
	s := New(1)
	if !s.TryAcquire() {
		t.Error("expected success")
	}
	if s.TryAcquire() {
		t.Error("expected failure")
	}
	s.Release()
	if !s.TryAcquire() {
		t.Error("expected success after release")
	}
	s.Release()
}

func TestSemaphore_Concurrent(t *testing.T) {
	s := New(5)
	var wg sync.WaitGroup
	var maxConcurrent int
	var mu sync.Mutex
	var current int
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Acquire(context.Background())
			defer s.Release()
			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			current--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if maxConcurrent > 5 {
		t.Errorf("expected max 5 concurrent, got %d", maxConcurrent)
	}
}

func TestSemaphore_Cap(t *testing.T) {
	s := New(10)
	if s.Cap() != 10 {
		t.Errorf("expected 10, got %d", s.Cap())
	}
}

func TestSemaphore_Zero(t *testing.T) {
	s := New(0)
	if s.Cap() != 1 {
		t.Error("expected min cap 1")
	}
}

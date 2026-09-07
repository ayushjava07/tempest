package lock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLock_Basic(t *testing.T) {
	l := New()
	if err := l.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !l.IsHeld() {
		t.Error("expected held")
	}
	l.Unlock()
	if l.IsHeld() {
		t.Error("expected not held")
	}
}

func TestLock_TryLock(t *testing.T) {
	l := New()
	if !l.TryLock() {
		t.Error("expected true")
	}
	if l.TryLock() {
		t.Error("expected false")
	}
	l.Unlock()
}

func TestLock_Contention(t *testing.T) {
	l := New()
	l.Lock(context.Background())
	var got atomic.Bool
	go func() {
		err := l.Lock(context.Background())
		got.Store(err == nil)
	}()
	time.Sleep(10 * time.Millisecond)
	l.Unlock()
	time.Sleep(10 * time.Millisecond)
	if !got.Load() {
		t.Error("expected waiter to get lock")
	}
}

func TestLock_ContextCancel(t *testing.T) {
	l := New()
	l.Lock(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	err := l.Lock(ctx)
	if err != context.Canceled {
		t.Errorf("expected Canceled, got %v", err)
	}
}

func TestLock_Waiters(t *testing.T) {
	l := New()
	l.Lock(context.Background())
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { _ = l.Lock(ctx1) }()
	go func() { _ = l.Lock(ctx2) }()
	time.Sleep(10 * time.Millisecond)
	if l.Waiters() != 2 {
		t.Errorf("expected 2 waiters, got %d", l.Waiters())
	}
	cancel1()
	cancel2()
	time.Sleep(10 * time.Millisecond)
}

func TestRWLock_Basic(t *testing.T) {
	rw := NewRWLock()
	if err := rw.RLock(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !rw.TryRLock() {
		t.Error("expected TryRLock success")
	}
	rw.RUnlock()
	rw.RUnlock()
}

func TestRWLock_WriteLock(t *testing.T) {
	rw := NewRWLock()
	if err := rw.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rw.TryRLock() {
		t.Error("expected TryRLock fail")
	}
	if rw.TryLock() {
		t.Error("expected TryLock fail")
	}
	rw.Unlock()
}

func TestRWLock_ReadersBlockWriter(t *testing.T) {
	rw := NewRWLock()
	rw.RLock(context.Background())
	var writerGot atomic.Bool
	go func() {
		err := rw.Lock(context.Background())
		writerGot.Store(err == nil)
	}()
	time.Sleep(10 * time.Millisecond)
	rw.RUnlock()
	time.Sleep(10 * time.Millisecond)
	if !writerGot.Load() {
		t.Error("expected writer to get lock")
	}
}

func TestRWLock_WriterBlocksReaders(t *testing.T) {
	rw := NewRWLock()
	rw.Lock(context.Background())
	var readerGot atomic.Bool
	go func() {
		err := rw.RLock(context.Background())
		readerGot.Store(err == nil)
	}()
	time.Sleep(10 * time.Millisecond)
	rw.Unlock()
	time.Sleep(10 * time.Millisecond)
	if !readerGot.Load() {
		t.Error("expected reader to get lock")
	}
}

func TestRWLock_ConcurrentReaders(t *testing.T) {
	rw := NewRWLock()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rw.RLock(context.Background())
			time.Sleep(10 * time.Millisecond)
			rw.RUnlock()
		}()
	}
	wg.Wait()
}

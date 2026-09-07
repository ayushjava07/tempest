package fencing

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestZombieWriteRejection reproduces the classic split-brain zombie write scenario
// and verifies that the storage layer rejects writes from superseded clients.
func TestZombieWriteRejection(t *testing.T) {
	gen := NewTokenGenerator()
	storage := NewFencedStorage()

	// 1. Worker-1 acquires lock and receives token 1
	tok1 := gen.AcquireToken("customer-balance", "worker-1", 10*time.Second)
	if tok1.Token != 1 {
		t.Fatalf("expected token 1, got %d", tok1.Token)
	}

	// 2. Worker-1 experiences simulated network partition / GC pause...
	// Meanwhile, lease expires or coordinator redistributes lock to Worker-2
	tok2 := gen.AcquireToken("customer-balance", "worker-2", 10*time.Second)
	if tok2.Token != 2 {
		t.Fatalf("expected token 2, got %d", tok2.Token)
	}

	// 3. Worker-2 performs state update with token 2
	err := storage.Put("customer-balance", []byte("$200.00"), tok2)
	if err != nil {
		t.Fatalf("worker-2 write failed: %v", err)
	}

	// 4. Worker-1 wakes up from pause and attempts zombie write with stale token 1
	errZombie := storage.Put("customer-balance", []byte("$100.00"), tok1)
	if errZombie == nil {
		t.Fatalf("expected zombie write to be rejected by storage!")
	}
	if !errors.Is(errZombie, ErrStaleFencingToken) {
		t.Fatalf("expected ErrStaleFencingToken, got %v", errZombie)
	}

	// 5. Verify storage integrity: value must remain $200.00 from token 2
	val, authoredTok, ok := storage.Get("customer-balance")
	if !ok {
		t.Fatalf("customer-balance missing")
	}
	if string(val) != "$200.00" {
		t.Fatalf("corrupted storage state! expected $200.00, got %s", string(val))
	}
	if authoredTok.Token != 2 {
		t.Fatalf("expected authored token 2, got %d", authoredTok.Token)
	}

	if storage.RejectedCount() != 1 {
		t.Fatalf("expected 1 rejected zombie write, got %d", storage.RejectedCount())
	}
}

func TestExpiredTokenRejection(t *testing.T) {
	gen := NewTokenGenerator()
	storage := NewFencedStorage()

	// Token with immediately expiring TTL (1 millisecond)
	tok := gen.AcquireToken("short-lease", "worker", 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	err := storage.Put("short-lease", []byte("stale-data"), tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestPreemptionCallbackDetection(t *testing.T) {
	gen := NewTokenGenerator()
	lm := NewLeaseManager(gen)

	tok1 := gen.AcquireToken("order-queue", "worker-1", 10*time.Second)

	var preemptionCalled atomic.Bool
	lm.OnPreemption("order-queue", func(newTok FencingToken) {
		preemptionCalled.Store(true)
	})

	// Worker-1 checks preemption before work: should be OK
	if err := lm.CheckPreemption(tok1); err != nil {
		t.Fatalf("unexpected preemption error: %v", err)
	}

	// Worker-2 supersedes with token 2
	tok2 := gen.AcquireToken("order-queue", "worker-2", 10*time.Second)
	lm.NotifyPreemption(tok2)

	// Worker-1 checks preemption again: should detect preemption!
	err := lm.CheckPreemption(tok1)
	if !errors.Is(err, ErrPreempted) {
		t.Fatalf("expected ErrPreempted, got %v", err)
	}

	if !preemptionCalled.Load() {
		t.Fatalf("expected preemption listener to be triggered")
	}
}

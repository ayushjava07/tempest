package lock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestLock_ExclusiveMutualExclusion(t *testing.T) {
	coord := NewCoordinator()
	ctx := context.Background()

	l1, err := coord.Acquire(ctx, "res-1", "alice", ModeExclusive, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("alice Acquire failed: %v", err)
	}
	if l1.FencingToken == 0 {
		t.Error("expected non-zero fencing token")
	}

	// Bob attempts to acquire with short timeout; must fail with context deadline exceeded
	bobCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err = coord.Acquire(bobCtx, "res-1", "bob", ModeExclusive, 500*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// Alice releases
	if err := coord.Release("res-1", "alice"); err != nil {
		t.Fatalf("alice Release failed: %v", err)
	}

	// Bob can now acquire
	l2, err := coord.Acquire(ctx, "res-1", "bob", ModeExclusive, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("bob Acquire failed: %v", err)
	}
	if l2.FencingToken <= l1.FencingToken {
		t.Errorf("expected monotonic fencing token increase: %d <= %d", l2.FencingToken, l1.FencingToken)
	}
	_ = coord.Release("res-1", "bob")
}

func TestLock_SharedConcurrency(t *testing.T) {
	coord := NewCoordinator()
	ctx := context.Background()

	// Alice and Bob can concurrently hold shared locks on res-shared
	l1, err := coord.Acquire(ctx, "res-shared", "alice", ModeShared, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("alice shared acquire failed: %v", err)
	}

	l2, err := coord.Acquire(ctx, "res-shared", "bob", ModeShared, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("bob shared acquire failed: %v", err)
	}

	if l1.FencingToken == 0 || l2.FencingToken == 0 {
		t.Error("expected valid fencing tokens")
	}

	// Charlie wants exclusive lock, should time out while shared locks exist
	charlieCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = coord.Acquire(charlieCtx, "res-shared", "charlie", ModeExclusive, 500*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline exceeded for exclusive waiter, got %v", err)
	}

	_ = coord.Release("res-shared", "alice")
	_ = coord.Release("res-shared", "bob")
}

func TestLock_DeadlockDetection(t *testing.T) {
	coord := NewCoordinator()
	ctx := context.Background()

	// Alice acquires res-A
	_, err := coord.Acquire(ctx, "res-A", "alice", ModeExclusive, 2*time.Second)
	if err != nil {
		t.Fatalf("alice acquire res-A failed: %v", err)
	}

	// Bob acquires res-B
	_, err = coord.Acquire(ctx, "res-B", "bob", ModeExclusive, 2*time.Second)
	if err != nil {
		t.Fatalf("bob acquire res-B failed: %v", err)
	}

	// Goroutine: Alice attempts to acquire res-B (will block waiting for Bob)
	aliceDone := make(chan error, 1)
	go func() {
		aliceCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		_, err := coord.Acquire(aliceCtx, "res-B", "alice", ModeExclusive, 2*time.Second)
		aliceDone <- err
	}()

	// Allow Alice to register wait in wait-for graph
	time.Sleep(30 * time.Millisecond)

	// Now Bob attempts to acquire res-A (creating cycle: Alice->Bob->Alice)
	// Bob must immediately detect deadlock!
	_, err = coord.Acquire(ctx, "res-A", "bob", ModeExclusive, 2*time.Second)
	if !errors.Is(err, ErrDeadlockDetected) {
		t.Fatalf("expected ErrDeadlockDetected for Bob, got %v", err)
	}

	// Clean up
	_ = coord.Release("res-A", "alice")
	_ = coord.Release("res-B", "bob")
}

func TestLock_RenewalAndExpiration(t *testing.T) {
	coord := NewCoordinator()
	ctx := context.Background()

	l, err := coord.Acquire(ctx, "res-temp", "alice", ModeExclusive, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Renew before expiry
	renewed, err := coord.Renew("res-temp", "alice", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Renew failed: %v", err)
	}
	if !renewed.ExpiresAt.After(l.ExpiresAt) {
		t.Errorf("renewed expiresAt %v not after initial %v", renewed.ExpiresAt, l.ExpiresAt)
	}

	// Non-owner cannot renew
	_, err = coord.Renew("res-temp", "bob", 200*time.Millisecond)
	if !errors.Is(err, ErrLockNotHeld) {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}

	_ = coord.Release("res-temp", "alice")
}

func TestLock_ConcurrentContentions(t *testing.T) {
	coord := NewCoordinator()
	concurrency := 10
	iterations := 10

	var wg sync.WaitGroup
	var counter int
	var counterMu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			owner := string(rune('A' + id))
			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				lease, err := coord.Acquire(ctx, "shared-counter", owner, ModeExclusive, 100*time.Millisecond)
				cancel()
				if err != nil {
					continue
				}

				counterMu.Lock()
				counter++
				counterMu.Unlock()

				_ = coord.Release("shared-counter", owner)
				_ = lease
			}
		}(i)
	}
	wg.Wait()
}

package lease

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCoordinator_AcquireAndRelease(t *testing.T) {
	c := NewCoordinator()
	ctx := context.Background()

	l, err := c.Acquire(ctx, "res-1", "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("failed to acquire lease: %v", err)
	}

	if l.FencingToken <= 0 {
		t.Errorf("expected positive fencing token, got %d", l.FencingToken)
	}

	if !c.Validate("res-1", "worker-a", l.FencingToken) {
		t.Errorf("expected lease to be valid")
	}

	// Worker B cannot acquire
	_, err = c.Acquire(ctx, "res-1", "worker-b", time.Minute)
	if err != ErrLeaseHeld {
		t.Fatalf("expected ErrLeaseHeld, got %v", err)
	}

	// Worker A releases
	if err := c.Release(ctx, "res-1", "worker-a", l.FencingToken); err != nil {
		t.Fatalf("failed to release lease: %v", err)
	}

	// Worker B can now acquire
	l2, err := c.Acquire(ctx, "res-1", "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("expected worker-b to acquire after release: %v", err)
	}
	if l2.FencingToken <= l.FencingToken {
		t.Errorf("expected monotonic fencing token increase: %d <= %d", l2.FencingToken, l.FencingToken)
	}
}

func TestCoordinator_Renew(t *testing.T) {
	c := NewCoordinator()
	ctx := context.Background()

	l, err := c.Acquire(ctx, "res-2", "worker-a", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	renewed, err := c.Renew(ctx, "res-2", "worker-a", l.FencingToken, time.Hour)
	if err != nil {
		t.Fatalf("failed to renew lease: %v", err)
	}

	if renewed.ExpiresAt.Before(time.Now().Add(30 * time.Minute)) {
		t.Errorf("expected lease to be extended by ~1 hour")
	}

	// Wrong token renewal fails
	_, err = c.Renew(ctx, "res-2", "worker-a", 999, time.Hour)
	if err != ErrFencingStale {
		t.Errorf("expected ErrFencingStale, got %v", err)
	}
}

func TestCoordinator_ConcurrentAcquisition(t *testing.T) {
	c := NewCoordinator()
	ctx := context.Background()

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := c.Acquire(ctx, "shared-res", "worker", time.Minute)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	if successCount == 0 {
		t.Errorf("expected at least one acquisition")
	}
}

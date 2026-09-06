package quota

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

func TestQuota_AcquireAndRelease(t *testing.T) {
	mgr := NewManager()
	ns := "tenant-a"
	mgr.SetLimits(ns, TenantLimits{
		MaxConcurrentRuns: 2,
		MaxQueueDepth:     10,
		MaxDefinitions:    5,
		MaxStepsPerSecond: 50,
	})

	ctx := context.Background()

	// Acquire 1 run
	r1, err := mgr.Acquire(ctx, ns, ResourceConcurrentRuns, 1)
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	usage := mgr.GetUsage(ns)
	if usage.ConcurrentRuns != 1 {
		t.Errorf("expected 1 run in usage, got %d", usage.ConcurrentRuns)
	}

	// Acquire 2nd run
	r2, err := mgr.Acquire(ctx, ns, ResourceConcurrentRuns, 1)
	if err != nil {
		t.Fatalf("second Acquire failed: %v", err)
	}

	// 3rd acquire must fail with ErrQuotaExceeded
	_, err = mgr.Acquire(ctx, ns, ResourceConcurrentRuns, 1)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}

	// Release r1
	r1.Release()
	usage = mgr.GetUsage(ns)
	if usage.ConcurrentRuns != 1 {
		t.Errorf("expected 1 run in usage after r1 release, got %d", usage.ConcurrentRuns)
	}

	// Now 3rd acquire can succeed
	r3, err := mgr.Acquire(ctx, ns, ResourceConcurrentRuns, 1)
	if err != nil {
		t.Fatalf("third Acquire after release failed: %v", err)
	}

	// Idempotent release
	r1.Release()
	r2.Release()
	r3.Release()

	usage = mgr.GetUsage(ns)
	if usage.ConcurrentRuns != 0 {
		t.Errorf("expected 0 runs after all released, got %d", usage.ConcurrentRuns)
	}
}

func TestQuota_SlidingWindowLimiter(t *testing.T) {
	limiter := NewSlidingWindowLimiter(5, 100*time.Millisecond, 5)
	now := time.Now()

	// First 5 should succeed
	for i := 0; i < 5; i++ {
		if !limiter.Allow(now) {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 6th request within window must be rejected
	if limiter.Allow(now) {
		t.Error("request 6 should be rejected")
	}

	// Advance time past the sliding window
	future := now.Add(150 * time.Millisecond)
	if !limiter.Allow(future) {
		t.Error("request in future window should be allowed")
	}
}

func TestQuota_ConcurrentAcquire(t *testing.T) {
	mgr := NewManager()
	ns := "tenant-concurrent"
	maxCap := 50
	mgr.SetLimits(ns, TenantLimits{
		MaxConcurrentRuns: maxCap,
		MaxQueueDepth:     100,
		MaxDefinitions:    100,
		MaxStepsPerSecond: 100,
	})

	ctx := context.Background()
	concurrency := 100
	var wg sync.WaitGroup
	var acquiredCount int
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := mgr.Acquire(ctx, ns, ResourceConcurrentRuns, 1)
			if err == nil {
				mu.Lock()
				acquiredCount++
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				res.Release()
			}
		}()
	}

	wg.Wait()

	usage := mgr.GetUsage(ns)
	if usage.ConcurrentRuns != 0 {
		t.Errorf("expected all reservations released (0 usage), got %d", usage.ConcurrentRuns)
	}
}

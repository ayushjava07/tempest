package tokenbucket

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBucket_TryAcquireAndRefill(t *testing.T) {
	cfg := Config{
		Capacity:   10,
		RefillRate: 100, // 100 tokens per sec -> 1 token per 10ms
	}
	b := NewBucket(cfg)

	// Available starts at capacity
	if avail := b.Available(); avail != 10 {
		t.Fatalf("expected 10 tokens initially, got %f", avail)
	}

	// Consume 8 tokens
	if !b.TryAcquire(8) {
		t.Fatalf("expected TryAcquire(8) to succeed")
	}

	// Try acquiring 5 tokens (should fail, only 2 left)
	if b.TryAcquire(5) {
		t.Fatalf("expected TryAcquire(5) to fail")
	}

	// Wait 50ms (refills ~5 tokens)
	time.Sleep(50 * time.Millisecond)

	// Now TryAcquire(5) should succeed
	if !b.TryAcquire(5) {
		t.Fatalf("expected TryAcquire(5) to succeed after refill")
	}
}

func TestBucket_AcquireBlockingAndContextCancel(t *testing.T) {
	cfg := Config{
		Capacity:   5,
		RefillRate: 10, // 10 tokens per sec
	}
	b := NewBucket(cfg)

	// Deplete bucket
	_ = b.TryAcquire(5)

	// Acquire with timeout that should cancel
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := b.Acquire(ctxTimeout, 5) // needs 500ms to refill 5 tokens
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestBucket_LeaseAndRelease(t *testing.T) {
	cfg := Config{
		Capacity:   10,
		RefillRate: 1, // very slow refill
	}
	b := NewBucket(cfg)

	ctx := context.Background()
	lease, err := b.Lease(ctx, "tenant-a", 6, 1*time.Second)
	if err != nil {
		t.Fatalf("Lease failed: %v", err)
	}

	if lease.Tokens != 6 {
		t.Fatalf("expected 6 leased tokens, got %f", lease.Tokens)
	}

	// Available tokens should drop to ~4
	avail := b.Available()
	if avail > 4.5 {
		t.Fatalf("expected ~4 tokens available during active lease, got %f", avail)
	}

	// Early explicit release
	if err := lease.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Tokens returned immediately
	availAfter := b.Available()
	if availAfter < 9.5 {
		t.Fatalf("expected ~10 tokens after release, got %f", availAfter)
	}
}

func TestBucket_LeaseExpiryReap(t *testing.T) {
	cfg := Config{
		Capacity:   10,
		RefillRate: 1,
	}
	b := NewBucket(cfg)

	ctx := context.Background()
	// Create short 30ms lease
	_, err := b.Lease(ctx, "tenant-dead-worker", 7, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("Lease failed: %v", err)
	}

	// Before expiration
	if reaped := b.ReapExpiredLeases(); reaped != 0 {
		t.Fatalf("expected 0 reaped leases, got %d", reaped)
	}

	// Wait for TTL to pass
	time.Sleep(50 * time.Millisecond)

	// Now reaper should clean up dead lease and restore tokens
	reaped := b.ReapExpiredLeases()
	if reaped != 1 {
		t.Fatalf("expected 1 reaped lease, got %d", reaped)
	}

	avail := b.Available()
	if avail < 9.5 {
		t.Fatalf("expected ~10 tokens after dead worker reap, got %f", avail)
	}
}

func TestMultiLimiter_TenantIsolation(t *testing.T) {
	limiter := NewMultiLimiter(Config{
		Capacity:   10,
		RefillRate: 10,
	})

	ctx := context.Background()

	// Exhaust tenant A
	for i := 0; i < 10; i++ {
		if err := limiter.Acquire(ctx, "tenant-A", 1); err != nil {
			t.Fatalf("acquire tenant A failed: %v", err)
		}
	}

	// Tenant B should still have full capacity
	_, availB, _, _ := limiter.Stats("tenant-B")
	if availB != 10 {
		t.Fatalf("expected tenant B to have full 10 tokens, got %f", availB)
	}
}

func TestMultiLimiter_ReaperLifecycle(t *testing.T) {
	limiter := NewMultiLimiter(Config{
		Capacity:   10,
		RefillRate: 10,
	})

	limiter.StartReaper(20 * time.Millisecond)

	ctx := context.Background()
	_, _ = limiter.Lease(ctx, "tenant-x", 5, 30*time.Millisecond)

	time.Sleep(60 * time.Millisecond)

	// Lease should have been reaped automatically
	_, _, _, activeLeases := limiter.Stats("tenant-x")
	if activeLeases != 0 {
		t.Fatalf("expected 0 active leases after automatic reaping, got %d", activeLeases)
	}

	limiter.Stop()
}

func TestMultiLimiter_Concurrency(t *testing.T) {
	limiter := NewMultiLimiter(Config{
		Capacity:   100,
		RefillRate: 1000,
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			tenant := "shared-tenant"
			for j := 0; j < 20; j++ {
				_ = limiter.Acquire(ctx, tenant, 1)
			}
		}(i)
	}

	wg.Wait()
}

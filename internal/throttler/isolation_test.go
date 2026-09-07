package throttler

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTenantQuotaIsolation(t *testing.T) {
	th := NewSlidingLogThrottler()

	// Tenant A: 5 ops / 100ms
	_ = th.SetQuota(ThrottleConfig{
		QuotaKey:  "tenant-A",
		RateLimit: 5,
		Window:    100 * time.Millisecond,
	})

	// Tenant B: 10 ops / 100ms
	_ = th.SetQuota(ThrottleConfig{
		QuotaKey:  "tenant-B",
		RateLimit: 10,
		Window:    100 * time.Millisecond,
	})

	// 1. Completely exhaust Tenant A
	for i := 0; i < 5; i++ {
		d, _ := th.Allow("tenant-A")
		if !d.Allowed {
			t.Fatalf("unexpected rejection in initial tenant-A calls")
		}
	}
	dA, _ := th.Allow("tenant-A")
	if dA.Allowed {
		t.Fatalf("tenant-A must be throttled")
	}

	// 2. Tenant B must still have full allowance (10 ops) regardless of tenant-A being saturated!
	for i := 0; i < 10; i++ {
		dB, err := th.Allow("tenant-B")
		if err != nil || !dB.Allowed {
			t.Fatalf("tenant-B call %d blocked by noisy neighbor tenant-A: %+v (err: %v)", i, dB, err)
		}
	}

	// 11th call for tenant-B is now throttled
	dBThrottled, _ := th.Allow("tenant-B")
	if dBThrottled.Allowed {
		t.Fatalf("tenant-B should be throttled on 11th call")
	}
}

func TestConcurrentMultiTenantAdmission(t *testing.T) {
	th := NewSlidingLogThrottler()

	const tenantCount = 5
	const opsPerTenant = 20

	for i := 1; i <= tenantCount; i++ {
		_ = th.SetQuota(ThrottleConfig{
			QuotaKey:  fmt.Sprintf("tenant-%d", i),
			RateLimit: opsPerTenant,
			Window:    500 * time.Millisecond,
		})
	}

	var wg sync.WaitGroup
	var acceptedTotal atomic.Uint64
	var rejectedTotal atomic.Uint64

	// Launch concurrent requests across tenants
	for i := 1; i <= tenantCount; i++ {
		wg.Add(1)
		go func(tenantID int) {
			defer wg.Done()
			key := fmt.Sprintf("tenant-%d", tenantID)

			// Try 30 requests (20 should succeed, 10 should be throttled)
			for req := 0; req < 30; req++ {
				d, err := th.Allow(key)
				if err == nil {
					if d.Allowed {
						acceptedTotal.Add(1)
					} else {
						rejectedTotal.Add(1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	expectedAccepted := uint64(tenantCount * opsPerTenant) // 5 * 20 = 100
	expectedRejected := uint64(tenantCount * 10)           // 5 * 10 = 50

	if acceptedTotal.Load() != expectedAccepted {
		t.Fatalf("expected %d total accepted, got %d", expectedAccepted, acceptedTotal.Load())
	}
	if rejectedTotal.Load() != expectedRejected {
		t.Fatalf("expected %d total rejected, got %d", expectedRejected, rejectedTotal.Load())
	}
}

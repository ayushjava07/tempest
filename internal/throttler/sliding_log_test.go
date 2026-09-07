package throttler

import (
	"testing"
	"time"
)

func TestSlidingLogQuotaEnforcement(t *testing.T) {
	th := NewSlidingLogThrottler()

	cfg := ThrottleConfig{
		QuotaKey:  "tenant-api",
		RateLimit: 5,
		Window:    100 * time.Millisecond,
	}

	if err := th.SetQuota(cfg); err != nil {
		t.Fatalf("set quota failed: %v", err)
	}

	// 1. First 5 calls should succeed
	for i := 0; i < 5; i++ {
		d, err := th.Allow("tenant-api")
		if err != nil {
			t.Fatalf("allow %d returned error: %v", i, err)
		}
		if !d.Allowed {
			t.Fatalf("expected allowed=true at call %d, remaining=%d", i, d.Remaining)
		}
		if d.Remaining != 4-i {
			t.Fatalf("expected remaining=%d, got %d", 4-i, d.Remaining)
		}
	}

	// 2. 6th call should be throttled
	dReject, err := th.Allow("tenant-api")
	if err != nil {
		t.Fatalf("allow returned error: %v", err)
	}
	if dReject.Allowed {
		t.Fatalf("expected 6th call to be rejected!")
	}
	if dReject.WaitDuration <= 0 {
		t.Fatalf("expected positive wait duration")
	}

	// 3. Wait for sliding window to elapse
	time.Sleep(110 * time.Millisecond)

	// Next call should succeed now
	dRecover, err := th.Allow("tenant-api")
	if err != nil || !dRecover.Allowed {
		t.Fatalf("expected call to succeed after window expiration, got allowed=%v", dRecover.Allowed)
	}
}

func TestAllowNMultiTokenConsumption(t *testing.T) {
	th := NewSlidingLogThrottler()

	cfg := ThrottleConfig{
		QuotaKey:  "batch-ops",
		RateLimit: 10,
		Window:    200 * time.Millisecond,
	}
	_ = th.SetQuota(cfg)

	// Consume 6 tokens
	d1, err := th.AllowN("batch-ops", 6)
	if err != nil || !d1.Allowed || d1.Remaining != 4 {
		t.Fatalf("consume 6 failed: %+v (err: %v)", d1, err)
	}

	// Try to consume 5 tokens (only 4 left) -> rejected
	d2, err := th.AllowN("batch-ops", 5)
	if err != nil || d2.Allowed {
		t.Fatalf("expected rejection on consume 5 when 4 left, got allowed=%v", d2.Allowed)
	}

	// Consume remaining 4 tokens -> succeeds
	d3, err := th.AllowN("batch-ops", 4)
	if err != nil || !d3.Allowed || d3.Remaining != 0 {
		t.Fatalf("consume 4 failed: %+v", d3)
	}
}

func TestBurstSmootherMicroLimiting(t *testing.T) {
	// Macro: 10 ops / 100ms, Micro: 3 ops / 30ms
	cfg := BurstSmootherConfig{
		QuotaKey:       "smooth-api",
		MacroRate:      10,
		MacroWindow:    100 * time.Millisecond,
		MicroBurstRate: 3,
		MicroWindow:    30 * time.Millisecond,
	}

	smoother, err := NewBurstSmoother(cfg)
	if err != nil {
		t.Fatalf("burst smoother error: %v", err)
	}

	// 3 rapid calls in 0ms: should pass micro-limit
	for i := 0; i < 3; i++ {
		d, err := smoother.Allow()
		if err != nil || !d.Allowed {
			t.Fatalf("call %d failed: %+v (err: %v)", i, d, err)
		}
	}

	// 4th call immediately: should be throttled by micro-burst window
	d4, err := smoother.Allow()
	if err != nil || d4.Allowed {
		t.Fatalf("expected 4th call throttled by micro-burst rate, got allowed=%v", d4.Allowed)
	}

	// Estimate wait for 5 queued items
	wait := smoother.EstimateWait(5)
	if wait <= 0 {
		t.Fatalf("expected positive estimated wait, got %s", wait)
	}
}

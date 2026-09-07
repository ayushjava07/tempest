package adaptive

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

func TestAdaptive_AcquireAndRelease(t *testing.T) {
	limiter := NewLimiter(Config{
		MinLimit:     2,
		MaxLimit:     10,
		InitialLimit: 5,
	})

	ctx := context.Background()
	tok, err := limiter.Acquire(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if limiter.InFlight() != 1 {
		t.Errorf("expected InFlight=1, got %d", limiter.InFlight())
	}

	tok.Release(nil)
	if limiter.InFlight() != 0 {
		t.Errorf("expected InFlight=0 after release, got %d", limiter.InFlight())
	}

	// Double release is idempotent
	tok.Release(nil)
	if limiter.InFlight() != 0 {
		t.Errorf("expected InFlight=0 after double release, got %d", limiter.InFlight())
	}
}

func TestAdaptive_TryAcquireOverloaded(t *testing.T) {
	limiter := NewLimiter(Config{
		MinLimit:     2,
		MaxLimit:     5,
		InitialLimit: 2,
	})

	t1, err := limiter.TryAcquire()
	if err != nil {
		t.Fatalf("t1 TryAcquire failed: %v", err)
	}
	t2, err := limiter.TryAcquire()
	if err != nil {
		t.Fatalf("t2 TryAcquire failed: %v", err)
	}

	// 3rd should fail since limit is 2
	_, err = limiter.TryAcquire()
	if !errors.Is(err, ErrOverloaded) {
		t.Errorf("expected ErrOverloaded, got %v", err)
	}

	t1.Release(nil)
	t2.Release(nil)
}

func TestAdaptive_LatencySpikeDecreasesLimit(t *testing.T) {
	limiter := NewLimiter(Config{
		MinLimit:     2,
		MaxLimit:     20,
		InitialLimit: 10,
		Smoothing:    0.5,
		Headroom:     0.1,
		RttTolerance: 1.1,
	})

	// Establish baseline low latency (10ms)
	for i := 0; i < 5; i++ {
		limiter.onSample(10*time.Millisecond, false)
	}

	initialLimit := limiter.CurrentLimit()

	// Inject sharp latency spike (200ms)
	for i := 0; i < 5; i++ {
		limiter.onSample(200*time.Millisecond, false)
	}

	decreasedLimit := limiter.CurrentLimit()
	if decreasedLimit >= initialLimit {
		t.Errorf("expected limit to decrease on latency spike: %d >= %d", decreasedLimit, initialLimit)
	}
}

func TestAdaptive_ErrorDecreasesLimit(t *testing.T) {
	limiter := NewLimiter(Config{
		MinLimit:     2,
		MaxLimit:     20,
		InitialLimit: 10,
	})

	initial := limiter.CurrentLimit()

	// Sample errors
	limiter.onSample(10*time.Millisecond, true)
	limiter.onSample(10*time.Millisecond, true)

	afterErrors := limiter.CurrentLimit()
	if afterErrors >= initial {
		t.Errorf("expected limit to decrease after errors: %d >= %d", afterErrors, initial)
	}
}

func TestAdaptive_Concurrency(t *testing.T) {
	limiter := NewLimiter(Config{
		MinLimit:     5,
		MaxLimit:     30,
		InitialLimit: 15,
	})

	concurrency := 20
	iterations := 10
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				tok, err := limiter.Acquire(ctx, 200*time.Millisecond)
				cancel()
				if err != nil {
					continue
				}
				time.Sleep(1 * time.Millisecond)
				tok.Release(nil)
			}
		}()
	}
	wg.Wait()

	if limiter.InFlight() != 0 {
		t.Errorf("expected InFlight=0 after all workers finish, got %d", limiter.InFlight())
	}
}

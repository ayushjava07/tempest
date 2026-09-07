package providers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSecretRotationAndCacheInvalidation(t *testing.T) {
	ctx := context.Background()
	backend := NewEnvProvider("TEST_")

	// Store initial secret
	_ = backend.PutSecret(ctx, "oauth.client_secret", []byte("old-secret-v1"))

	// Initialize short TTL cached provider
	cached, err := NewCachedProvider(backend, 50*time.Millisecond, 0.0, nil)
	if err != nil {
		t.Fatalf("init cached provider failed: %v", err)
	}

	sec, err := cached.GetSecret(ctx, "oauth.client_secret")
	if err != nil || string(sec.Value) != "old-secret-v1" {
		t.Fatalf("unexpected initial value: %s, err: %v", string(sec.Value), err)
	}

	// 1. External secret rotation occurs in backend
	_ = backend.PutSecret(ctx, "oauth.client_secret", []byte("new-rotated-secret-v2"))

	// Cache should still hold old secret before TTL expiration
	secCached, _ := cached.GetSecret(ctx, "oauth.client_secret")
	if string(secCached.Value) != "old-secret-v1" {
		t.Fatalf("expected cached secret before TTL expiry, got %s", string(secCached.Value))
	}

	// 2. Explicit Invalidate forces immediate pickup of rotated secret
	cached.Invalidate("oauth.client_secret")
	secRotated, err := cached.GetSecret(ctx, "oauth.client_secret")
	if err != nil || string(secRotated.Value) != "new-rotated-secret-v2" {
		t.Fatalf("expected new rotated secret after invalidation, got %s (err: %v)", string(secRotated.Value), err)
	}

	// 3. Test TTL expiration pickup
	_ = backend.PutSecret(ctx, "oauth.client_secret", []byte("newest-secret-v3"))
	time.Sleep(65 * time.Millisecond)

	secTTL, err := cached.GetSecret(ctx, "oauth.client_secret")
	if err != nil || string(secTTL.Value) != "newest-secret-v3" {
		t.Fatalf("expected secret to update after TTL expiration, got %s", string(secTTL.Value))
	}
}

func TestConcurrentReadersDuringRotation(t *testing.T) {
	ctx := context.Background()
	backend := NewEnvProvider("CONC_")
	_ = backend.PutSecret(ctx, "token", []byte("tok-initial"))

	cached, err := NewCachedProvider(backend, 20*time.Millisecond, 0.0, nil)
	if err != nil {
		t.Fatalf("cached provider error: %v", err)
	}

	stopChan := make(chan struct{})
	var wg sync.WaitGroup
	var readErrors atomic.Uint64

	// Spawn 10 concurrent reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					return
				default:
					sec, err := cached.GetSecret(ctx, "token")
					if err != nil || len(sec.Value) == 0 {
						readErrors.Add(1)
					}
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	// Periodically rotate secret 5 times
	for round := 1; round <= 5; round++ {
		time.Sleep(10 * time.Millisecond)
		_ = cached.PutSecret(ctx, "token", []byte("tok-updated"))
	}

	close(stopChan)
	wg.Wait()

	if readErrors.Load() > 0 {
		t.Fatalf("encountered %d read errors during concurrent rotation", readErrors.Load())
	}
}

package fencing

import (
	"sync"
	"testing"
	"time"
)

func TestStrictlyMonotonicSequentialTokens(t *testing.T) {
	gen := NewTokenGenerator()

	var lastSeq uint64 = 0
	for i := 0; i < 100; i++ {
		tok := gen.AcquireToken("order-101", "worker-A", 10*time.Second)
		if tok.Token <= lastSeq {
			t.Fatalf("expected monotonic increase, got %d <= last %d", tok.Token, lastSeq)
		}
		if tok.Token != uint64(i+1) {
			t.Fatalf("expected token %d, got %d", i+1, tok.Token)
		}
		lastSeq = tok.Token
	}

	latest, ok := gen.LatestToken("order-101")
	if !ok || latest.Token != 100 {
		t.Fatalf("expected latest token 100, got %+v", latest)
	}
}

func TestConcurrentMonotonicTokenGeneration(t *testing.T) {
	gen := NewTokenGenerator()

	const routines = 20
	const perRoutine = 50
	totalExpected := routines * perRoutine

	var wg sync.WaitGroup
	var mu sync.Mutex
	seenTokens := make(map[uint64]bool)

	for r := 0; r < routines; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perRoutine; i++ {
				tok := gen.AcquireToken("shared-resource", "worker", 5*time.Second)
				mu.Lock()
				if seenTokens[tok.Token] {
					t.Errorf("duplicate token detected: %d", tok.Token)
				}
				seenTokens[tok.Token] = true
				mu.Unlock()
			}
		}(r)
	}

	wg.Wait()

	if len(seenTokens) != totalExpected {
		t.Fatalf("expected %d unique tokens, got %d", totalExpected, len(seenTokens))
	}

	// Verify all tokens from 1 to totalExpected are present
	for i := uint64(1); i <= uint64(totalExpected); i++ {
		if !seenTokens[i] {
			t.Fatalf("missing token sequence %d", i)
		}
	}
}

func TestLeaseExtension(t *testing.T) {
	gen := NewTokenGenerator()
	lm := NewLeaseManager(gen)

	tok := gen.AcquireToken("res-1", "worker-1", 50*time.Millisecond)
	oldExpiry := tok.ExpiresAt

	time.Sleep(10 * time.Millisecond)

	extTok, err := lm.ExtendLease(tok, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("extend lease failed: %v", err)
	}

	if !extTok.ExpiresAt.After(oldExpiry) {
		t.Fatalf("expected new expiry %s to be after old %s", extTok.ExpiresAt, oldExpiry)
	}

	if extTok.Token != tok.Token {
		t.Fatalf("lease extension must preserve token number")
	}
}

package fencing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestContextBindingAndDerivation(t *testing.T) {
	ctx := context.Background()

	// 1. Missing token
	_, err := RequireFencingToken(ctx)
	if !errors.Is(err, ErrMissingContextToken) {
		t.Fatalf("expected ErrMissingContextToken, got %v", err)
	}

	tok := FencingToken{
		Resource:  "wf-order-99",
		Token:     42,
		Owner:     "worker-main",
		Epoch:     1,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}

	// 2. Bind to context
	ctxWithTok := WithFencingToken(ctx, tok)
	retrieved, ok := FromContext(ctxWithTok)
	if !ok || retrieved.Token != 42 {
		t.Fatalf("retrieved token mismatch: %+v", retrieved)
	}

	// 3. Derive child token for parallel sub-step
	subCtx, childTok, err := WithDerivedSubStep(ctxWithTok, "branch-A")
	if err != nil {
		t.Fatalf("failed to derive sub-step: %v", err)
	}
	if childTok.Owner != "worker-main/branch-A" {
		t.Fatalf("unexpected child owner: %s", childTok.Owner)
	}
	if childTok.Token != 42 {
		t.Fatalf("child token must inherit token sequence 42")
	}

	subRetrieved, ok := FromContext(subCtx)
	if !ok || subRetrieved.Owner != "worker-main/branch-A" {
		t.Fatalf("unexpected sub context token: %+v", subRetrieved)
	}
}

func TestParallelSubStepsWithFencedStorage(t *testing.T) {
	gen := NewTokenGenerator()
	storage := NewFencedStorage()

	parentTok := gen.AcquireToken("batch-job", "orchestrator", 10*time.Second)
	ctx := WithFencingToken(context.Background(), parentTok)

	const parallelSteps = 10
	var wg sync.WaitGroup
	var errCount sync.Map

	for i := 0; i < parallelSteps; i++ {
		wg.Add(1)
		go func(stepIdx int) {
			defer wg.Done()
			subStepID := fmt.Sprintf("step-%02d", stepIdx)

			_, childTok, err := WithDerivedSubStep(ctx, subStepID)
			if err != nil {
				errCount.Store(subStepID, err)
				return
			}

			key := fmt.Sprintf("partition-%02d", stepIdx)
			val := []byte(fmt.Sprintf("result-from-%s", subStepID))

			if writeErr := storage.Put(key, val, childTok); writeErr != nil {
				errCount.Store(subStepID, writeErr)
			}
		}(i)
	}

	wg.Wait()

	// Check for any errors
	hasErr := false
	errCount.Range(func(k, v any) bool {
		t.Errorf("error in %v: %v", k, v)
		hasErr = true
		return true
	})
	if hasErr {
		t.Fatalf("parallel sub-step writes failed")
	}

	// Verify all 10 partitions written
	for i := 0; i < parallelSteps; i++ {
		key := fmt.Sprintf("partition-%02d", i)
		val, authored, ok := storage.Get(key)
		if !ok || authored.Token != parentTok.Token {
			t.Fatalf("partition %s missing or wrong token: %v (authored=%+v)", key, ok, authored)
		}
		expectedVal := fmt.Sprintf("result-from-step-%02d", i)
		if string(val) != expectedVal {
			t.Fatalf("unexpected value for %s: got %s, want %s", key, string(val), expectedVal)
		}
	}
}

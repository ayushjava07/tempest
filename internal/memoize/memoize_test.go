package memoize

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMemoize_CanonicalKeyComputation(t *testing.T) {
	// Map 1: a then b
	m1 := map[string]any{
		"a": "first",
		"b": 123,
	}

	// Map 2: b then a
	m2 := map[string]any{
		"b": 123,
		"a": "first",
	}

	k1 := ComputeKey("default", "wf-etl", "step-extract", m1)
	k2 := ComputeKey("default", "wf-etl", "step-extract", m2)

	if k1 != k2 {
		t.Errorf("expected identical keys for maps with different key order: %s != %s", k1, k2)
	}

	// Different input should yield different key
	m3 := map[string]any{"a": "different"}
	k3 := ComputeKey("default", "wf-etl", "step-extract", m3)
	if k1 == k3 {
		t.Error("expected different keys for different inputs")
	}
}

func TestMemoize_PutAndGet(t *testing.T) {
	c := NewCache(100)
	ctx := context.Background()

	input := map[string]any{"query": "SELECT * FROM users"}
	output := map[string]any{"rows": 42}

	key := c.Put(ctx, "prod", "analytics", "sql_step", input, output, 1*time.Minute)

	got, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got["rows"] != 42 {
		t.Errorf("expected rows=42, got %v", got["rows"])
	}

	// Verify deep copy: mutating got should not mutate cached entry
	got["rows"] = 999
	got2, _ := c.Get(ctx, key)
	if got2["rows"] != 42 {
		t.Errorf("cached entry was mutated in place: %v", got2["rows"])
	}

	stats := c.Stats()
	if stats.Hits != 2 || stats.Misses != 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestMemoize_TTLExpiration(t *testing.T) {
	c := NewCache(100)
	ctx := context.Background()

	key := c.Put(ctx, "default", "wf", "s1", map[string]any{"x": 1}, map[string]any{"y": 2}, 30*time.Millisecond)

	// Immediate get works
	_, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("expected immediate hit, got %v", err)
	}

	// Sleep past TTL
	time.Sleep(50 * time.Millisecond)

	_, err = c.Get(ctx, key)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("expected ErrExpired after TTL, got %v", err)
	}
}

func TestMemoize_Invalidation(t *testing.T) {
	c := NewCache(100)
	ctx := context.Background()

	k1 := c.Put(ctx, "ns-1", "wf-1", "step-a", map[string]any{"p": 1}, map[string]any{"r": 1}, 0)
	k2 := c.Put(ctx, "ns-1", "wf-1", "step-b", map[string]any{"p": 2}, map[string]any{"r": 2}, 0)
	k3 := c.Put(ctx, "ns-2", "wf-2", "step-c", map[string]any{"p": 3}, map[string]any{"r": 3}, 0)

	// Invalidate step-a
	del := c.InvalidateStep("ns-1", "wf-1", "step-a")
	if del != 1 {
		t.Errorf("expected 1 deleted entry for step-a, got %d", del)
	}
	if _, err := c.Get(ctx, k1); !errors.Is(err, ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss for invalidated k1, got %v", err)
	}

	// Invalidate entire namespace ns-1
	delNs := c.InvalidateNamespace("ns-1")
	if delNs != 1 {
		t.Errorf("expected 1 remaining entry deleted for ns-1, got %d", delNs)
	}
	if _, err := c.Get(ctx, k2); !errors.Is(err, ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss for invalidated k2, got %v", err)
	}

	// k3 in ns-2 must remain
	if _, err := c.Get(ctx, k3); err != nil {
		t.Errorf("expected k3 to remain valid, got %v", err)
	}
}

func TestMemoize_Concurrency(t *testing.T) {
	c := NewCache(500)
	concurrency := 10
	iterations := 20
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				in := map[string]any{"val": fmt.Sprintf("in-%d-%d", id, j)}
				out := map[string]any{"res": fmt.Sprintf("out-%d-%d", id, j)}
				k := c.Put(context.Background(), "ns", "wf", "s", in, out, 1*time.Minute)
				_, _ = c.Get(context.Background(), k)
			}
		}(i)
	}
	wg.Wait()

	stats := c.Stats()
	if stats.Hits == 0 {
		t.Errorf("expected non-zero hits in concurrent test")
	}
}

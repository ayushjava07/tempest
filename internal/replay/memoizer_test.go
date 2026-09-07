package replay

import (
	"fmt"
	"testing"
)

func TestStreamMemoizerIntervalAndLookup(t *testing.T) {
	memo := NewStreamMemoizer(10, 50)

	// Populate states across sequences
	for seq := uint64(1); seq <= 100; seq++ {
		st := map[string]string{
			"counter": fmt.Sprintf("%d", seq),
			"val":     fmt.Sprintf("v-%d", seq),
		}
		memo.MaybeMemoize(seq, st, false)
	}

	// Should have checkpoints at 10, 20, 30, ..., 100 -> 10 checkpoints
	if memo.Len() != 10 {
		t.Fatalf("expected 10 checkpoints, got %d", memo.Len())
	}

	// Query targetSeq = 25 -> nearest should be 20
	seq, st, ok := memo.FindNearestCheckpoint(25)
	if !ok || seq != 20 {
		t.Fatalf("expected nearest seq 20, got %d (ok=%v)", seq, ok)
	}
	if st["counter"] != "20" {
		t.Fatalf("expected counter 20, got %s", st["counter"])
	}

	// Query targetSeq = 5 -> should be false (earliest is 10)
	_, _, ok = memo.FindNearestCheckpoint(5)
	if ok {
		t.Fatalf("expected ok=false for targetSeq < earliest checkpoint")
	}

	// Query targetSeq = 100 -> nearest should be 100
	seq, st, ok = memo.FindNearestCheckpoint(100)
	if !ok || seq != 100 {
		t.Fatalf("expected nearest seq 100, got %d", seq)
	}

	// Test forced memoization
	forceState := map[string]string{"urgent": "true"}
	memo.MaybeMemoize(105, forceState, true)
	seq, st, ok = memo.FindNearestCheckpoint(106)
	if !ok || seq != 105 {
		t.Fatalf("expected forced checkpoint 105, got %d", seq)
	}
	if st["urgent"] != "true" {
		t.Fatalf("expected urgent=true")
	}
}

func TestStreamMemoizerCapacityEviction(t *testing.T) {
	memo := NewStreamMemoizer(5, 3)

	memo.MaybeMemoize(5, map[string]string{"k": "5"}, false)
	memo.MaybeMemoize(10, map[string]string{"k": "10"}, false)
	memo.MaybeMemoize(15, map[string]string{"k": "15"}, false)

	if memo.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", memo.Len())
	}

	// Add 4th checkpoint -> seq 5 should be evicted
	memo.MaybeMemoize(20, map[string]string{"k": "20"}, false)
	if memo.Len() != 3 {
		t.Fatalf("expected capacity capped at 3, got %d", memo.Len())
	}

	// Seq 5 should no longer be found
	_, _, ok := memo.FindNearestCheckpoint(8)
	if ok {
		t.Fatalf("expected seq 5 to have been evicted")
	}

	// Seq 10 should be found
	seq, _, ok := memo.FindNearestCheckpoint(12)
	if !ok || seq != 10 {
		t.Fatalf("expected nearest seq 10, got %d", seq)
	}
}

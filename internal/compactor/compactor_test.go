package compactor

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCompactor_PutGetDelete(t *testing.T) {
	store := New(DefaultConfig())
	defer store.Close()

	// Put
	_, err := store.Put("user:101", []byte("Alice"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	val, found, err := store.Get("user:101")
	if err != nil || !found {
		t.Fatalf("expected to find user:101, found=%v err=%v", found, err)
	}
	if !bytes.Equal(val, []byte("Alice")) {
		t.Fatalf("expected Alice, got %s", string(val))
	}

	// Delete
	_, err = store.Delete("user:101")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Get after Delete
	_, found, err = store.Get("user:101")
	if err != nil || found {
		t.Fatalf("expected user:101 not to be found after delete")
	}
}

func TestCompactor_FlushAndCompact(t *testing.T) {
	// Force flush every 3 operations
	store := New(Config{FlushThreshold: 3})
	defer store.Close()

	// Batch 1: keys A, B, C (causes flush to Table 1)
	_, _ = store.Put("A", []byte("1"))
	_, _ = store.Put("B", []byte("1"))
	_, _ = store.Put("C", []byte("1"))

	// Batch 2: update A, delete B, add D (causes flush to Table 2)
	_, _ = store.Put("A", []byte("2")) // updated
	_, _ = store.Delete("B")           // tombstone
	_, _ = store.Put("D", []byte("1"))

	liveKeys, numTables := store.Stats()
	if numTables != 2 {
		t.Fatalf("expected 2 snapshot tables before compaction, got %d", numTables)
	}
	if liveKeys != 3 { // A (value 2), C (value 1), D (value 1)
		t.Fatalf("expected 3 live keys, got %d", liveKeys)
	}

	// Verify values before compaction
	valA, _, _ := store.Get("A")
	if !bytes.Equal(valA, []byte("2")) {
		t.Fatalf("expected A=2, got %s", string(valA))
	}
	_, foundB, _ := store.Get("B")
	if foundB {
		t.Fatalf("expected B to be deleted")
	}

	// Run compaction
	err := store.Compact()
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	// After compaction: exactly 1 merged table
	liveKeysAfter, numTablesAfter := store.Stats()
	if numTablesAfter != 1 {
		t.Fatalf("expected 1 merged table after compaction, got %d", numTablesAfter)
	}
	if liveKeysAfter != 3 {
		t.Fatalf("expected 3 live keys after compaction, got %d", liveKeysAfter)
	}

	// Re-verify values after compaction
	valA, _, _ = store.Get("A")
	if !bytes.Equal(valA, []byte("2")) {
		t.Fatalf("expected A=2 after compaction, got %s", string(valA))
	}
	_, foundB, _ = store.Get("B")
	if foundB {
		t.Fatalf("expected B to remain deleted after compaction")
	}
}

func TestCompactor_Concurrency(t *testing.T) {
	store := New(Config{FlushThreshold: 10})
	defer store.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				val := []byte(fmt.Sprintf("val-%d", j))
				_, _ = store.Put(key, val)
				_, _, _ = store.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Run compaction concurrently safe
	err := store.Compact()
	if err != nil {
		t.Fatalf("concurrent compaction failed: %v", err)
	}
}

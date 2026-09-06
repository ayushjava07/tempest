package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCheckpoint_SaveAndRestore(t *testing.T) {
	store := NewMemoryStore()
	engine := NewEngine(store)
	ctx := context.Background()

	stateData := []byte(`{"step":"step-1","data":{"counter":42}}`)
	cp, err := engine.CheckpointRun(ctx, "default", "run-101", "step-1", 1, stateData, map[string]string{"env": "test"})
	if err != nil {
		t.Fatalf("CheckpointRun failed: %v", err)
	}

	if cp.Checksum != ComputeChecksum(stateData) {
		t.Errorf("checksum mismatch: %d != %d", cp.Checksum, ComputeChecksum(stateData))
	}

	restored, err := engine.RestoreLatest(ctx, "default", "run-101")
	if err != nil {
		t.Fatalf("RestoreLatest failed: %v", err)
	}

	if restored.Sequence != 1 || string(restored.State) != string(stateData) {
		t.Errorf("unexpected restored state: %s", string(restored.State))
	}
}

func TestCheckpoint_CRC32CorruptionDetection(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	stateData := []byte(`{"valid":true}`)
	cp := &Checkpoint{
		ID:        "cp-bad",
		Namespace: "default",
		RunID:     "run-corrupt",
		Sequence:  1,
		State:     stateData,
		Checksum:  ComputeChecksum(stateData) + 1, // Deliberate mismatch
	}

	err := store.Save(ctx, cp)
	if !errors.Is(err, ErrCorruptCheckpoint) {
		t.Fatalf("expected ErrCorruptCheckpoint, got %v", err)
	}
}

func TestCheckpoint_SequenceMonotonicity(t *testing.T) {
	store := NewMemoryStore()
	engine := NewEngine(store)
	ctx := context.Background()

	_, err := engine.CheckpointRun(ctx, "default", "run-seq", "step-1", 10, []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("first checkpoint failed: %v", err)
	}

	// Lower or equal sequence must be rejected
	_, err = engine.CheckpointRun(ctx, "default", "run-seq", "step-2", 5, []byte(`{}`), nil)
	if !errors.Is(err, ErrNonMonotonicSeq) {
		t.Fatalf("expected ErrNonMonotonicSeq for decreasing sequence, got %v", err)
	}

	_, err = engine.CheckpointRun(ctx, "default", "run-seq", "step-2", 10, []byte(`{}`), nil)
	if !errors.Is(err, ErrNonMonotonicSeq) {
		t.Fatalf("expected ErrNonMonotonicSeq for identical sequence, got %v", err)
	}
}

func TestCheckpoint_Pruning(t *testing.T) {
	store := NewMemoryStore()
	engine := NewEngine(store)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		_, err := engine.CheckpointRun(ctx, "default", "run-prune", fmt.Sprintf("step-%d", i), uint64(i), []byte(fmt.Sprintf("state-%d", i)), nil)
		if err != nil {
			t.Fatalf("checkpoint %d failed: %v", i, err)
		}
	}

	list, _ := store.List(ctx, "default", "run-prune")
	if len(list) != 5 {
		t.Fatalf("expected 5 checkpoints, got %d", len(list))
	}

	// Prune keeping latest 2
	pruned, err := store.Prune(ctx, "default", "run-prune", 2)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if pruned != 3 {
		t.Errorf("expected 3 pruned checkpoints, got %d", pruned)
	}

	remaining, _ := store.List(ctx, "default", "run-prune")
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining checkpoints, got %d", len(remaining))
	}
	if remaining[0].Sequence != 4 || remaining[1].Sequence != 5 {
		t.Errorf("unexpected remaining sequences: %d, %d", remaining[0].Sequence, remaining[1].Sequence)
	}
}

func TestCheckpoint_ConcurrentCheckpoints(t *testing.T) {
	store := NewMemoryStore()
	engine := NewEngine(store)
	concurrency := 10

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runID := fmt.Sprintf("run-concurrent-%d", id)
			for j := 1; j <= 5; j++ {
				_, err := engine.CheckpointRun(context.Background(), "default", runID, fmt.Sprintf("step-%d", j), uint64(j), []byte("payload"), nil)
				if err != nil {
					t.Errorf("concurrent checkpoint failed: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
}

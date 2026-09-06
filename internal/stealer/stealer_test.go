package stealer

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDeque_LIFOAndFIFO(t *testing.T) {
	d := NewDeque(10)

	_ = d.PushBottom(Task{ID: "task-1"})
	_ = d.PushBottom(Task{ID: "task-2"})
	_ = d.PushBottom(Task{ID: "task-3"})

	// Steal from top should yield task-1 (FIFO)
	stolen, err := d.StealTop()
	if err != nil {
		t.Fatalf("StealTop failed: %v", err)
	}
	if stolen.ID != "task-1" {
		t.Errorf("expected stolen task-1, got %s", stolen.ID)
	}

	// Pop from bottom should yield task-3 (LIFO)
	local, err := d.PopBottom()
	if err != nil {
		t.Fatalf("PopBottom failed: %v", err)
	}
	if local.ID != "task-3" {
		t.Errorf("expected local task-3, got %s", local.ID)
	}

	// Remaining should be task-2
	rem, err := d.PopBottom()
	if err != nil || rem.ID != "task-2" {
		t.Errorf("expected remaining task-2, got %v (%v)", rem.ID, err)
	}

	// Now empty
	_, err = d.PopBottom()
	if !errors.Is(err, ErrQueueEmpty) {
		t.Errorf("expected ErrQueueEmpty, got %v", err)
	}
}

func TestDeque_CapacityLimits(t *testing.T) {
	d := NewDeque(2)
	_ = d.PushBottom(Task{ID: "t1"})
	_ = d.PushBottom(Task{ID: "t2"})

	err := d.PushBottom(Task{ID: "t3"})
	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestPool_WorkStealingDistribution(t *testing.T) {
	var processedCount atomic.Int64

	handler := func(ctx context.Context, t Task) error {
		processedCount.Add(1)
		time.Sleep(2 * time.Millisecond)
		return nil
	}

	numWorkers := 4
	totalTasks := 40
	pool := NewPool(numWorkers, 100, handler)
	pool.Start()
	defer pool.Stop()

	// Submit ALL tasks exclusively to worker 0
	for i := 0; i < totalTasks; i++ {
		_ = pool.Submit(0, Task{ID: fmt.Sprintf("task-%d", i)})
	}

	// Wait for completion
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if processedCount.Load() >= int64(totalTasks) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if processedCount.Load() < int64(totalTasks) {
		t.Fatalf("expected all %d tasks processed, got %d", totalTasks, processedCount.Load())
	}

	stats := pool.Stats()
	if stats.StolenCount == 0 {
		t.Errorf("expected peer workers to steal tasks, got StolenCount=0")
	}
}

func TestPool_Lifecycle(t *testing.T) {
	pool := NewPool(2, 50, nil)
	pool.Start()
	pool.Start() // Idempotent start

	time.Sleep(20 * time.Millisecond)

	pool.Stop()
	pool.Stop() // Idempotent stop
}

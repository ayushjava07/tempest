package deadlock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDeadlock_NoCycle(t *testing.T) {
	d := New()

	d.RegisterTask("T1", "wf1", 10, time.Now())
	d.RegisterTask("T2", "wf1", 10, time.Now())

	_, _ = d.AcquireLock("T1", "lock-A")
	_, _ = d.AcquireLock("T2", "lock-B")

	cycles := d.DetectDeadlocks()
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %v", cycles)
	}
}

func TestDeadlock_TwoTaskCycle(t *testing.T) {
	d := New()

	now := time.Now()
	d.RegisterTask("T1", "wf-1", 10, now.Add(-10*time.Second))
	d.RegisterTask("T2", "wf-1", 5, now) // lower priority, younger

	// T1 acquires A, T2 acquires B
	_, _ = d.AcquireLock("T1", "lock-A")
	_, _ = d.AcquireLock("T2", "lock-B")

	// T1 waits for B (held by T2)
	_, _ = d.AcquireLock("T1", "lock-B")

	// T2 waits for A (held by T1) -> creates deadlock
	_, _ = d.AcquireLock("T2", "lock-A")

	cycles := d.DetectDeadlocks()
	if len(cycles) == 0 {
		t.Fatalf("expected deadlock cycle to be detected")
	}

	// Test victim selection by lowest priority
	victim, err := d.SelectVictim(cycles[0], VictimLowestPriority)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}
	if victim != "T2" {
		t.Fatalf("expected T2 (priority 5) to be chosen as victim over T1 (priority 10), got %s", victim)
	}

	// Test resolution
	var abortedTask string
	resolved := d.ResolveDeadlocks(VictimLowestPriority, func(taskID string) {
		abortedTask = taskID
	})

	if resolved != 1 || abortedTask != "T2" {
		t.Fatalf("expected 1 resolution aborting T2, got resolved=%d aborted=%s", resolved, abortedTask)
	}

	// Graph should now be acyclic
	cyclesAfter := d.DetectDeadlocks()
	if len(cyclesAfter) != 0 {
		t.Fatalf("expected 0 cycles after resolution, got %v", cyclesAfter)
	}
}

func TestDeadlock_ThreeTaskCircular(t *testing.T) {
	d := New()

	now := time.Now()
	d.RegisterTask("T1", "wf-1", 10, now)
	d.RegisterTask("T2", "wf-1", 20, now)
	d.RegisterTask("T3", "wf-1", 30, now)

	_, _ = d.AcquireLock("T1", "res-1")
	_, _ = d.AcquireLock("T2", "res-2")
	_, _ = d.AcquireLock("T3", "res-3")

	// T1 waits for res-2
	_, _ = d.AcquireLock("T1", "res-2")
	// T2 waits for res-3
	_, _ = d.AcquireLock("T2", "res-3")
	// T3 waits for res-1
	_, _ = d.AcquireLock("T3", "res-1")

	cycles := d.DetectDeadlocks()
	if len(cycles) == 0 {
		t.Fatalf("expected 3-way circular deadlock")
	}

	// Lowest priority (T1 with priority 10)
	victim, err := d.SelectVictim(cycles[0], VictimLowestPriority)
	if err != nil || victim != "T1" {
		t.Fatalf("expected T1 victim, got %s (err: %v)", victim, err)
	}
}

func TestDeadlock_SweeperLifecycle(t *testing.T) {
	d := New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var aborted atomic.Int32
	d.StartSweeper(ctx, 20*time.Millisecond, VictimLowestPriority, func(taskID string) {
		aborted.Add(1)
	})

	d.RegisterTask("A", "wf", 1, time.Now())
	d.RegisterTask("B", "wf", 2, time.Now())
	_, _ = d.AcquireLock("A", "L1")
	_, _ = d.AcquireLock("B", "L2")
	_, _ = d.AcquireLock("A", "L2")
	_, _ = d.AcquireLock("B", "L1")

	time.Sleep(60 * time.Millisecond)

	if aborted.Load() != 1 {
		t.Fatalf("expected 1 task aborted by sweeper, got %d", aborted.Load())
	}

	d.Stop()
}

func TestDeadlock_Concurrency(t *testing.T) {
	d := New()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			taskID := "task-" + string(rune('A'+idx))
			d.RegisterTask(taskID, "wf", idx, time.Now())
			_, _ = d.AcquireLock(taskID, "common-resource")
			_ = d.ReleaseLock(taskID, "common-resource")
			d.UnregisterTask(taskID)
		}(i)
	}

	wg.Wait()
}

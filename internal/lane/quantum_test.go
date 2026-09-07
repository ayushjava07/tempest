package lane

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestDRRDeficitAccumulationOverMultipleRounds verifies that when a task's cost
// exceeds a single round's quantum, deficit credits accumulate across rounds until
// the task can be served.
func TestDRRDeficitAccumulationOverMultipleRounds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Lane A: quantum = 2, task cost = 5 (requires 3 rounds: 2 -> 4 -> 6 >= 5)
	// Lane B: quantum = 1, task cost = 1
	laneA, _ := NewLane(LaneConfig{ID: "lane-A", Priority: 10, Quantum: 2, MaxCapacity: 10})
	laneB, _ := NewLane(LaneConfig{ID: "lane-B", Priority: 10, Quantum: 1, MaxCapacity: 10})

	sched, err := NewDRRScheduler([]*Lane{laneA, laneB})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}

	_ = sched.Enqueue(&Task{ID: "heavy-task-A", LaneID: "lane-A", Cost: 5})
	_ = sched.Enqueue(&Task{ID: "light-task-B1", LaneID: "lane-B", Cost: 1})
	_ = sched.Enqueue(&Task{ID: "light-task-B2", LaneID: "lane-B", Cost: 1})
	_ = sched.Enqueue(&Task{ID: "light-task-B3", LaneID: "lane-B", Cost: 1})

	// Round 1: Lane A deficit=2 (<5, skipped), Lane B deficit=1 -> serves B1
	t1, err := sched.ScheduleNext(ctx)
	if err != nil || t1.ID != "light-task-B1" {
		t.Fatalf("expected light-task-B1 in round 1, got %v (err: %v)", t1, err)
	}

	// Round 2: Lane A deficit=4 (<5, skipped), Lane B deficit=1 -> serves B2
	t2, err := sched.ScheduleNext(ctx)
	if err != nil || t2.ID != "light-task-B2" {
		t.Fatalf("expected light-task-B2 in round 2, got %v (err: %v)", t2, err)
	}

	// Round 3: Lane A deficit=6 (>=5, SERVED!) -> serves heavy-task-A
	t3, err := sched.ScheduleNext(ctx)
	if err != nil || t3.ID != "heavy-task-A" {
		t.Fatalf("expected heavy-task-A in round 3, got %v (err: %v)", t3, err)
	}

	// Next should be remaining B3
	t4, err := sched.ScheduleNext(ctx)
	if err != nil || t4.ID != "light-task-B3" {
		t.Fatalf("expected light-task-B3, got %v (err: %v)", t4, err)
	}

	if sched.TotalTasks() != 0 {
		t.Fatalf("expected all tasks drained, remaining: %d", sched.TotalTasks())
	}
}

// TestDRRHeavyLoadMultiCostDraining tests draining thousands of varied cost tasks.
func TestDRRHeavyLoadMultiCostDraining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	l1, _ := NewLane(LaneConfig{ID: "l1", Priority: 1, Quantum: 10, MaxCapacity: 5000})
	l2, _ := NewLane(LaneConfig{ID: "l2", Priority: 2, Quantum: 20, MaxCapacity: 5000})

	sched, err := NewDRRScheduler([]*Lane{l1, l2})
	if err != nil {
		t.Fatalf("scheduler init failed: %v", err)
	}

	const taskCount = 1000
	for i := 0; i < taskCount; i++ {
		cost1 := (i % 5) + 1 // cost 1..5
		cost2 := (i % 7) + 1 // cost 1..7
		_ = sched.Enqueue(&Task{ID: fmt.Sprintf("l1-%d", i), LaneID: "l1", Cost: cost1})
		_ = sched.Enqueue(&Task{ID: fmt.Sprintf("l2-%d", i), LaneID: "l2", Cost: cost2})
	}

	totalExpected := taskCount * 2
	if sched.TotalTasks() != totalExpected {
		t.Fatalf("expected %d total tasks, got %d", totalExpected, sched.TotalTasks())
	}

	dispatched := 0
	for dispatched < totalExpected {
		task, err := sched.ScheduleNext(ctx)
		if err != nil {
			t.Fatalf("ScheduleNext failed at %d: %v", dispatched, err)
		}
		if task == nil {
			t.Fatalf("received nil task at %d", dispatched)
		}
		dispatched++
	}

	if sched.TotalTasks() != 0 {
		t.Fatalf("expected queue completely drained, got %d", sched.TotalTasks())
	}
}

package lane

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDRRFairDispatchRatio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lHigh, _ := NewLane(LaneConfig{ID: "high", Priority: 30, Quantum: 3, MaxCapacity: 100})
	lMed, _ := NewLane(LaneConfig{ID: "med", Priority: 20, Quantum: 2, MaxCapacity: 100})
	lLow, _ := NewLane(LaneConfig{ID: "low", Priority: 10, Quantum: 1, MaxCapacity: 100})

	sched, err := NewDRRScheduler([]*Lane{lHigh, lMed, lLow})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}

	// Enqueue 30 tasks to each lane with Cost=1
	for i := 0; i < 30; i++ {
		_ = sched.Enqueue(&Task{ID: fmt.Sprintf("h-%d", i), LaneID: "high", Cost: 1})
		_ = sched.Enqueue(&Task{ID: fmt.Sprintf("m-%d", i), LaneID: "med", Cost: 1})
		_ = sched.Enqueue(&Task{ID: fmt.Sprintf("l-%d", i), LaneID: "low", Cost: 1})
	}

	counts := make(map[string]int)
	// Dispatch 60 tasks (10 complete rounds: 10 * (3+2+1) = 60)
	for i := 0; i < 60; i++ {
		task, err := sched.ScheduleNext(ctx)
		if err != nil {
			t.Fatalf("ScheduleNext failed at %d: %v", i, err)
		}
		counts[task.LaneID]++
	}

	// Under 3:2:1 quantum, 60 tasks should distribute 30 high, 20 med, 10 low
	if counts["high"] != 30 {
		t.Fatalf("expected 30 high-priority tasks dispatched, got %d", counts["high"])
	}
	if counts["med"] != 20 {
		t.Fatalf("expected 20 med-priority tasks dispatched, got %d", counts["med"])
	}
	if counts["low"] != 10 {
		t.Fatalf("expected 10 low-priority tasks dispatched, got %d", counts["low"])
	}
}

func TestAntiStarvationPreemption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// High lane has large quantum, Low lane has 25ms starvation threshold
	lHigh, _ := NewLane(LaneConfig{ID: "high", Priority: 100, Quantum: 100, MaxCapacity: 1000})
	lLow, _ := NewLane(LaneConfig{
		ID:                  "low",
		Priority:            1,
		Quantum:             1,
		MaxCapacity:         100,
		StarvationThreshold: 25 * time.Millisecond,
	})

	sched, err := NewDRRScheduler([]*Lane{lHigh, lLow})
	if err != nil {
		t.Fatalf("scheduler init failed: %v", err)
	}

	// Enqueue a single low priority task
	_ = sched.Enqueue(&Task{ID: "starving-task", LaneID: "low", Cost: 1})

	// Wait past starvation threshold
	time.Sleep(35 * time.Millisecond)

	// Now flood high-priority queue with 20 tasks
	for i := 0; i < 20; i++ {
		_ = sched.Enqueue(&Task{ID: fmt.Sprintf("high-%d", i), LaneID: "high", Cost: 1})
	}

	// First scheduled task MUST be the starving low-priority task boosted by anti-starvation defense
	task, err := sched.ScheduleNext(ctx)
	if err != nil {
		t.Fatalf("ScheduleNext failed: %v", err)
	}

	if task.ID != "starving-task" {
		t.Fatalf("expected starving-task to preempt high-priority tasks, got %s", task.ID)
	}
	if !task.AgeBoosted {
		t.Fatalf("expected AgeBoosted=true on starved task")
	}
}

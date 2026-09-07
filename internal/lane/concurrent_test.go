package lane

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentEnqueueAcross10Lanes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numLanes = 10
	lanes := make([]*Lane, numLanes)
	for i := 0; i < numLanes; i++ {
		l, err := NewLane(LaneConfig{
			ID:          fmt.Sprintf("lane-%02d", i),
			Priority:    (i + 1) * 10,
			Quantum:     i + 1,
			MaxCapacity: 5000,
		})
		if err != nil {
			t.Fatalf("failed to create lane %d: %v", i, err)
		}
		lanes[i] = l
	}

	sched, err := NewDRRScheduler(lanes)
	if err != nil {
		t.Fatalf("scheduler error: %v", err)
	}

	bpController := NewBackpressureController(lanes, 0.70, 0.90)
	tuner := NewLatencyTuner(lanes, 0.2, 4)

	const producers = 10
	const tasksPerProducer = 100
	totalTasksToProduce := producers * tasksPerProducer

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	var enqueuedTotal atomic.Uint64
	var dequeuedTotal atomic.Uint64

	// Launch 10 producers concurrently
	for p := 0; p < producers; p++ {
		producerWg.Add(1)
		go func(prodID int) {
			defer producerWg.Done()
			for i := 0; i < tasksPerProducer; i++ {
				laneID := fmt.Sprintf("lane-%02d", (prodID+i)%numLanes)

				_ = bpController.Check(laneID)

				task := &Task{
					ID:      fmt.Sprintf("task-p%d-%d", prodID, i),
					LaneID:  laneID,
					Cost:    1,
					Payload: []byte("sample-work"),
				}

				if err := sched.Enqueue(task); err == nil {
					enqueuedTotal.Add(1)
				}
			}
		}(p)
	}

	// Launch 4 concurrent consumer workers
	for c := 0; c < 4; c++ {
		consumerWg.Add(1)
		go func(workerID int) {
			defer consumerWg.Done()
			for {
				if dequeuedTotal.Load() >= uint64(totalTasksToProduce) {
					return
				}

				task, err := sched.ScheduleNext(ctx)
				if err != nil {
					return
				}
				if task != nil {
					dequeuedTotal.Add(1)
					tuner.RecordLatency(task.LaneID, 5*time.Millisecond)
				}
			}
		}(c)
	}

	producerWg.Wait()
	consumerWg.Wait()

	if enqueuedTotal.Load() != uint64(totalTasksToProduce) {
		t.Fatalf("expected %d enqueued, got %d", totalTasksToProduce, enqueuedTotal.Load())
	}
	if dequeuedTotal.Load() != uint64(totalTasksToProduce) {
		t.Fatalf("expected %d dequeued, got %d", totalTasksToProduce, dequeuedTotal.Load())
	}

	metrics := tuner.Metrics()
	if len(metrics) != numLanes {
		t.Fatalf("expected metrics for %d lanes, got %d", numLanes, len(metrics))
	}
}

package e2e_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/barrier"
)

// Chunk represents a segment of input data to be mapped
type Chunk struct {
	ID    int
	Lines []string
}

// MapResult represents partial aggregation from a mapper
type MapResult struct {
	ChunkID   int
	WordCount map[string]int
}

func TestE2E_DynamicFanoutMapReduceBarrier(t *testing.T) {
	const numChunks = 16
	chunks := make([]Chunk, numChunks)
	for i := 0; i < numChunks; i++ {
		chunks[i] = Chunk{
			ID: i,
			Lines: []string{
				fmt.Sprintf("item-%d-alpha item-%d-beta", i, i),
				fmt.Sprintf("common-word item-%d-gamma", i),
				"common-word common-word",
			},
		}
	}

	// Step 1: Initialize barrier for mappers + reducer trigger
	b, err := barrier.NewBarrier(numChunks)
	if err != nil {
		t.Fatalf("failed to create barrier: %v", err)
	}

	resultsMu := sync.Mutex{}
	mapResults := make([]MapResult, 0, numChunks)
	var mapperCompletedCount int32
	var reducerStarted int32

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(numChunks)

	// Step 2: Fan out mapper goroutines
	for i := 0; i < numChunks; i++ {
		chunk := chunks[i]
		partyID := fmt.Sprintf("mapper-%02d", chunk.ID)

		go func(c Chunk, pid string) {
			defer wg.Done()

			// Perform mapping / tokenizing
			wc := make(map[string]int)
			for _, line := range c.Lines {
				for _, word := range splitWords(line) {
					wc[word]++
				}
			}

			resultsMu.Lock()
			mapResults = append(mapResults, MapResult{ChunkID: c.ID, WordCount: wc})
			resultsMu.Unlock()

			atomic.AddInt32(&mapperCompletedCount, 1)

			// Arrive at barrier and await all other mappers
			isLast, err := b.ArriveAndWait(ctx, pid)
			if err != nil {
				t.Errorf("mapper %s failed at barrier: %v", pid, err)
				return
			}

			if isLast {
				// The last arriving mapper can signal reducer or start reduce phase
				atomic.StoreInt32(&reducerStarted, 1)
			}
		}(chunk, partyID)
	}

	wg.Wait()

	// Step 3: Reducer verification
	if atomic.LoadInt32(&mapperCompletedCount) != int32(numChunks) {
		t.Fatalf("expected %d mappers completed, got %d", numChunks, atomic.LoadInt32(&mapperCompletedCount))
	}

	if atomic.LoadInt32(&reducerStarted) != 1 {
		t.Fatalf("expected last mapper to trip barrier and trigger reducer")
	}

	// Reduce phase: aggregate word counts across all chunks
	finalCounts := make(map[string]int)
	for _, res := range mapResults {
		for w, count := range res.WordCount {
			finalCounts[w] += count
		}
	}

	// Each chunk has 3 occurrences of "common-word" -> 3 * numChunks
	expectedCommon := 3 * numChunks
	if finalCounts["common-word"] != expectedCommon {
		t.Fatalf("expected %d occurrences of 'common-word', got %d", expectedCommon, finalCounts["common-word"])
	}

	// Verify unique items
	for i := 0; i < numChunks; i++ {
		alpha := fmt.Sprintf("item-%d-alpha", i)
		if finalCounts[alpha] != 1 {
			t.Errorf("expected count 1 for %s, got %d", alpha, finalCounts[alpha])
		}
	}
}

func TestE2E_MapReduceBarrierFailurePropagation(t *testing.T) {
	const numMappers = 8
	b, err := barrier.NewBarrier(numMappers)
	if err != nil {
		t.Fatalf("failed to create barrier: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(numMappers)

	var barrierErrors int32

	for i := 0; i < numMappers; i++ {
		mapperID := fmt.Sprintf("failing-mapper-%02d", i)
		isFailingNode := (i == 4)

		go func(id string, shouldFail bool) {
			defer wg.Done()

			if shouldFail {
				// Simulate failure before arriving, breaking the barrier
				time.Sleep(50 * time.Millisecond)
				b.Break(fmt.Errorf("fatal OOM error on node %s", id))
				return
			}

			_, err := b.ArriveAndWait(ctx, id)
			if err != nil {
				atomic.AddInt32(&barrierErrors, 1)
			}
		}(mapperID, isFailingNode)
	}

	wg.Wait()

	// All waiting mappers should have received the barrier break error
	if atomic.LoadInt32(&barrierErrors) == 0 {
		t.Fatalf("expected waiting mappers to be aborted with error when barrier broke")
	}
}

func splitWords(s string) []string {
	var words []string
	start := -1
	for i, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if start >= 0 {
				words = append(words, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		words = append(words, s[start:])
	}
	return words
}

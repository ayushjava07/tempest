package barrier

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBarrier_SynchronousRendezvous(t *testing.T) {
	const threshold = 5
	b, err := NewBarrier(threshold)
	if err != nil {
		t.Fatalf("NewBarrier failed: %v", err)
	}

	var lastCount atomic.Int32
	var releasedCount atomic.Int32
	var wg sync.WaitGroup

	ctx := context.Background()

	for i := 0; i < threshold; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			isLast, err := b.ArriveAndWait(ctx, fmt.Sprintf("party-%d", id))
			if err != nil {
				t.Errorf("party %d failed: %v", id, err)
				return
			}
			if isLast {
				lastCount.Add(1)
			}
			releasedCount.Add(1)
		}(i)
	}

	wg.Wait()

	if releasedCount.Load() != threshold {
		t.Fatalf("expected all %d parties released, got %d", threshold, releasedCount.Load())
	}
	if lastCount.Load() != 1 {
		t.Fatalf("expected exactly 1 party to trip barrier, got %d", lastCount.Load())
	}
}

func TestBarrier_CyclicGenerations(t *testing.T) {
	const threshold = 3
	b, _ := NewBarrier(threshold)
	ctx := context.Background()

	// Generation 1
	var wg sync.WaitGroup
	for i := 0; i < threshold; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := b.ArriveAndWait(ctx, fmt.Sprintf("gen1-%d", id))
			if err != nil {
				t.Errorf("gen 1 failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	// Generation 2 immediately after
	for i := 0; i < threshold; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := b.ArriveAndWait(ctx, fmt.Sprintf("gen2-%d", id))
			if err != nil {
				t.Errorf("gen 2 failed: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestBarrier_TimeoutAndBreak(t *testing.T) {
	b, _ := NewBarrier(3)

	var errReceived atomic.Int32
	var wg sync.WaitGroup

	// Party 1 waits with timeout 30ms
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		_, err := b.ArriveAndWait(ctx, "party-1")
		if err != nil {
			errReceived.Add(1)
		}
	}()

	// Party 2 waits with long context
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		_, err := b.ArriveAndWait(ctx, "party-2")
		if err != nil {
			errReceived.Add(1)
		}
	}()

	wg.Wait()

	// When party 1 timed out, the barrier was broken, unblocking party 2 with error
	if errReceived.Load() != 2 {
		t.Fatalf("expected both parties to fail when barrier broke, got %d", errReceived.Load())
	}
}

func TestBarrier_Coordinator(t *testing.T) {
	coord := NewCoordinator()

	b1, err := coord.GetOrCreate("scatter-gather-1", 4)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	b2, _ := coord.GetOrCreate("scatter-gather-1", 4)
	if b1 != b2 {
		t.Fatalf("expected singleton barrier for same name")
	}

	coord.Remove("scatter-gather-1")
	if b1.ArrivedCount() != 0 {
		t.Fatalf("expected clean remove")
	}
}

func TestBarrier_Concurrency(t *testing.T) {
	coord := NewCoordinator()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(barrierIdx int) {
			defer wg.Done()
			name := fmt.Sprintf("barrier-%d", barrierIdx)
			b, _ := coord.GetOrCreate(name, 4)
			ctx := context.Background()

			var subWg sync.WaitGroup
			for j := 0; j < 4; j++ {
				subWg.Add(1)
				go func(partyIdx int) {
					defer subWg.Done()
					_, _ = b.ArriveAndWait(ctx, fmt.Sprintf("p-%d", partyIdx))
				}(j)
			}
			subWg.Wait()
		}(i)
	}

	wg.Wait()
}

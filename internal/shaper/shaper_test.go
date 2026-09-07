package shaper

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestShaper_PriorityOrdering(t *testing.T) {
	cfg := Config{
		Capacity:   10,
		LeakRate:   1000, // fast leak for ordering test
		DropPolicy: DropNewest,
	}
	s := New(cfg)
	defer s.Close()

	ctx := context.Background()

	// Enqueue Normal, Low, High
	_ = s.Enqueue(ctx, Packet{ID: "norm-1", Priority: PriorityNormal})
	_ = s.Enqueue(ctx, Packet{ID: "low-1", Priority: PriorityLow})
	_ = s.Enqueue(ctx, Packet{ID: "high-1", Priority: PriorityHigh})
	_ = s.Enqueue(ctx, Packet{ID: "norm-2", Priority: PriorityNormal})

	// Expected drain order: high-1, norm-1, norm-2, low-1
	p1, _ := s.Next(ctx)
	p2, _ := s.Next(ctx)
	p3, _ := s.Next(ctx)
	p4, _ := s.Next(ctx)

	if p1.ID != "high-1" {
		t.Fatalf("expected high-1 first, got %s", p1.ID)
	}
	if p2.ID != "norm-1" {
		t.Fatalf("expected norm-1 second (FIFO among normal), got %s", p2.ID)
	}
	if p3.ID != "norm-2" {
		t.Fatalf("expected norm-2 third, got %s", p3.ID)
	}
	if p4.ID != "low-1" {
		t.Fatalf("expected low-1 last, got %s", p4.ID)
	}
}

func TestShaper_DropPolicies(t *testing.T) {
	t.Run("DropNewest", func(t *testing.T) {
		s := New(Config{Capacity: 2, LeakRate: 100, DropPolicy: DropNewest})
		defer s.Close()
		ctx := context.Background()

		_ = s.Enqueue(ctx, Packet{ID: "1"})
		_ = s.Enqueue(ctx, Packet{ID: "2"})
		err := s.Enqueue(ctx, Packet{ID: "3"})
		if err != ErrQueueFull {
			t.Fatalf("expected ErrQueueFull on DropNewest, got: %v", err)
		}
	})

	t.Run("DropOldest", func(t *testing.T) {
		s := New(Config{Capacity: 2, LeakRate: 1000, DropPolicy: DropOldest})
		defer s.Close()
		ctx := context.Background()

		_ = s.Enqueue(ctx, Packet{ID: "1", Priority: PriorityLow})
		_ = s.Enqueue(ctx, Packet{ID: "2", Priority: PriorityNormal})
		// Enqueuing 3 should drop "1" (lowest priority oldest)
		err := s.Enqueue(ctx, Packet{ID: "3", Priority: PriorityHigh})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		p1, _ := s.Next(ctx)
		p2, _ := s.Next(ctx)
		if p1.ID != "3" || p2.ID != "2" {
			t.Fatalf("expected [3, 2], got [%s, %s]", p1.ID, p2.ID)
		}
	})
}

func TestShaper_PacingDelay(t *testing.T) {
	// Leak rate = 20 packets/sec -> 50ms per packet
	s := New(Config{Capacity: 5, LeakRate: 20, DropPolicy: DropNewest})
	defer s.Close()
	ctx := context.Background()

	_ = s.Enqueue(ctx, Packet{ID: "p1"})
	_ = s.Enqueue(ctx, Packet{ID: "p2"})

	start := time.Now()
	_, _ = s.Next(ctx)
	_, _ = s.Next(ctx)
	elapsed := time.Since(start)

	// 2 packets * 50ms = ~100ms
	if elapsed < 80*time.Millisecond {
		t.Fatalf("pacing too fast, expected >= 80ms, got %v", elapsed)
	}
}

func TestShaper_StatsAndClose(t *testing.T) {
	s := New(Config{Capacity: 5, LeakRate: 1000, DropPolicy: DropNewest})
	ctx := context.Background()

	_ = s.Enqueue(ctx, Packet{ID: "a"})
	_ = s.Enqueue(ctx, Packet{ID: "b"})
	_, _ = s.Next(ctx)

	depth, enq, drain, drop := s.Stats()
	if depth != 1 || enq != 2 || drain != 1 || drop != 0 {
		t.Fatalf("unexpected stats: depth=%d enq=%d drain=%d drop=%d", depth, enq, drain, drop)
	}

	s.Close()
	err := s.Enqueue(ctx, Packet{ID: "c"})
	if err != ErrShaperClosed {
		t.Fatalf("expected ErrShaperClosed, got %v", err)
	}
}

func TestShaper_Concurrency(t *testing.T) {
	s := New(Config{Capacity: 100, LeakRate: 1000, DropPolicy: DropOldest})
	defer s.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	// 5 producers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = s.Enqueue(ctx, Packet{Priority: Priority(j % 3)})
			}
		}(i)
	}

	// 2 consumers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_, _ = s.Next(ctx)
			}
		}()
	}

	wg.Wait()
}

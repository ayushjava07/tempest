package eventbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTopicMatching(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		matched bool
	}{
		{"workflow.order.completed", "workflow.order.completed", true},
		{"workflow.order.completed", "workflow.order.failed", false},
		{"workflow.*.completed", "workflow.order.completed", true},
		{"workflow.*.completed", "workflow.payment.completed", true},
		{"workflow.*.completed", "workflow.order.sub.completed", false},
		{"workflow.#", "workflow.order.completed", true},
		{"workflow.#", "workflow.order.step.failed.retry", true},
		{"workflow.#.completed", "workflow.order.stage1.completed", true},
		{"#", "any.topic.structure", true},
		{"events.*", "events", false},
		{"events.*", "events.order", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			got := MatchTopic(tt.pattern, tt.topic)
			if got != tt.matched {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, got, tt.matched)
			}
		})
	}
}

func TestEventBus_BroadcastPublish(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Stop()

	var count1, count2 atomic.Int32

	sub1, err := bus.Subscribe("workflow.*.done", func(ctx context.Context, msg Message) error {
		count1.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe 1 failed: %v", err)
	}

	sub2, err := bus.Subscribe("workflow.#", func(ctx context.Context, msg Message) error {
		count2.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe 2 failed: %v", err)
	}

	ctx := context.Background()
	_ = bus.Publish(ctx, "workflow.payment.done", []byte("ok"), nil)

	time.Sleep(50 * time.Millisecond)

	if count1.Load() != 1 {
		t.Fatalf("expected sub1 to receive 1 message, got %d", count1.Load())
	}
	if count2.Load() != 1 {
		t.Fatalf("expected sub2 to receive 1 message, got %d", count2.Load())
	}

	// Unsubscribe sub1
	_ = bus.Unsubscribe(sub1)

	_ = bus.Publish(ctx, "workflow.inventory.done", []byte("ok"), nil)
	time.Sleep(50 * time.Millisecond)

	if count1.Load() != 1 {
		t.Fatalf("sub1 should not receive after unsubscribe, got %d", count1.Load())
	}
	if count2.Load() != 2 {
		t.Fatalf("sub2 should receive second message, got %d", count2.Load())
	}
	_ = sub2
}

func TestEventBus_ConsumerGroupLoadBalancing(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Stop()

	var worker1, worker2 atomic.Int32

	// Two workers in the same consumer group "workers-group"
	_, _ = bus.SubscribeGroup("workers-group", "tasks.#", func(ctx context.Context, msg Message) error {
		worker1.Add(1)
		return nil
	})
	_, _ = bus.SubscribeGroup("workers-group", "tasks.#", func(ctx context.Context, msg Message) error {
		worker2.Add(1)
		return nil
	})

	ctx := context.Background()
	numMessages := 20
	for i := 0; i < numMessages; i++ {
		_ = bus.Publish(ctx, "tasks.execute", []byte("payload"), nil)
	}

	time.Sleep(100 * time.Millisecond)

	total := worker1.Load() + worker2.Load()
	if total != int32(numMessages) {
		t.Fatalf("expected exactly %d messages processed across group, got %d", numMessages, total)
	}

	// Verify both workers handled roughly half
	if worker1.Load() == 0 || worker2.Load() == 0 {
		t.Fatalf("expected load balancing between workers, got w1=%d w2=%d", worker1.Load(), worker2.Load())
	}
}

func TestEventBus_DLQAndRetries(t *testing.T) {
	bus := New(Config{
		QueueCapacity: 64,
		WorkerCount:   2,
		MaxRetries:    2,
		RetryBackoff:  5 * time.Millisecond,
	})
	defer bus.Stop()

	var attempts atomic.Int32
	failingErr := errors.New("database connection refused")

	_, _ = bus.Subscribe("orders.create", func(ctx context.Context, msg Message) error {
		attempts.Add(1)
		return failingErr
	})

	_ = bus.Publish(context.Background(), "orders.create", []byte("order-100"), nil)

	time.Sleep(100 * time.Millisecond)

	// Initial attempt + 2 retries = 3 attempts total
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", attempts.Load())
	}

	dlq := bus.DeadLetters()
	if len(dlq) != 1 {
		t.Fatalf("expected 1 dead-letter entry, got %d", len(dlq))
	}
	if !errors.Is(dlq[0].Err, failingErr) {
		t.Fatalf("expected DLQ error to match %v, got %v", failingErr, dlq[0].Err)
	}
}

func TestEventBus_Concurrency(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Stop()

	var received atomic.Int64
	_, _ = bus.Subscribe("stream.#", func(ctx context.Context, msg Message) error {
		received.Add(1)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < 25; j++ {
				_ = bus.Publish(ctx, "stream.item", []byte("data"), nil)
			}
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if received.Load() != 500 {
		t.Fatalf("expected 500 received messages, got %d", received.Load())
	}
}

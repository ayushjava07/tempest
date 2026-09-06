package events

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestBus_PublishSubscribe(t *testing.T) {
	bus := NewBus(100)
	var received atomic.Int32
	bus.Subscribe("test", func(ctx context.Context, e Event) error {
		received.Add(1)
		return nil
	})
	err := bus.Publish(context.Background(), Event{Type: "test"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if received.Load() != 1 {
		t.Errorf("expected 1 handler call, got %d", received.Load())
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus(100)
	var count atomic.Int32
	bus.Subscribe("event", func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	})
	bus.Subscribe("event", func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	})
	_ = bus.Publish(context.Background(), Event{Type: "event"})
	if count.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", count.Load())
	}
}

func TestBus_HandlerError(t *testing.T) {
	bus := NewBus(100)
	bus.Subscribe("fail", func(ctx context.Context, e Event) error {
		return fmt.Errorf("handler error")
	})
	err := bus.Publish(context.Background(), Event{Type: "fail"})
	if err == nil {
		t.Error("expected error from handler")
	}
}

func TestBus_EnqueueDrain(t *testing.T) {
	bus := NewBus(100)
	var count atomic.Int32
	bus.Subscribe("queued", func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	})
	bus.Enqueue(Event{Type: "queued"})
	bus.Enqueue(Event{Type: "queued"})
	if bus.QueueSize() != 2 {
		t.Errorf("expected queue size 2, got %d", bus.QueueSize())
	}
	drained := bus.Drain(context.Background())
	if drained != 2 {
		t.Errorf("expected 2 drained, got %d", drained)
	}
	if bus.QueueSize() != 0 {
		t.Error("expected empty queue after drain")
	}
}

func TestBus_QueueOverflow(t *testing.T) {
	bus := NewBus(3)
	for i := 0; i < 5; i++ {
		bus.Enqueue(Event{Type: "overflow"})
	}
	if bus.QueueSize() != 3 {
		t.Errorf("expected queue size 3, got %d", bus.QueueSize())
	}
}

func TestBus_SubscriberCount(t *testing.T) {
	bus := NewBus(100)
	bus.Subscribe("a", func(ctx context.Context, e Event) error { return nil })
	bus.Subscribe("a", func(ctx context.Context, e Event) error { return nil })
	if bus.SubscriberCount("a") != 2 {
		t.Errorf("expected 2 subscribers, got %d", bus.SubscriberCount("a"))
	}
	if bus.SubscriberCount("b") != 0 {
		t.Error("expected 0 subscribers for unsubscribed event")
	}
}

func TestBus_NoSubscribers(t *testing.T) {
	bus := NewBus(100)
	err := bus.Publish(context.Background(), Event{Type: "nobody-listens"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestBus_Timestamp(t *testing.T) {
	bus := NewBus(100)
	var ts time.Time
	bus.Subscribe("check", func(ctx context.Context, e Event) error {
		ts = e.Timestamp
		return nil
	})
	_ = bus.Publish(context.Background(), Event{Type: "check"})
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

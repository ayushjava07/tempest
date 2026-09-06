package outbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type mockPublisher struct {
	mu        sync.Mutex
	published []Message
	failCount map[string]int
	failErr   error
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{
		failCount: make(map[string]int),
	}
}

func (m *mockPublisher) Publish(ctx context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if remaining := m.failCount[msg.ID]; remaining > 0 {
		m.failCount[msg.ID]--
		if m.failErr != nil {
			return m.failErr
		}
		return errors.New("simulated transient publish failure")
	}

	m.published = append(m.published, msg)
	return nil
}

func TestOutbox_StageAndPublish(t *testing.T) {
	store := NewMemoryStore()
	pub := newMockPublisher()

	proc := NewProcessor(store, pub, ProcessorConfig{
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		Concurrency:  2,
	})
	proc.Start()
	defer proc.Stop()

	ctx := context.Background()
	msg := Message{
		ID:      "msg-1",
		Topic:   "workflow.events",
		Key:     "wf-100",
		Payload: []byte(`{"status":"SUCCEEDED"}`),
	}

	if err := store.Stage(ctx, msg); err != nil {
		t.Fatalf("Stage failed: %v", err)
	}

	// Wait for processor to dispatch
	deadline := time.Now().Add(2 * time.Second)
	var stored Message
	for time.Now().Before(deadline) {
		m, ok := store.Get("msg-1")
		if ok && m.Status == StatusPublished {
			stored = m
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if stored.Status != StatusPublished {
		t.Fatalf("expected status PUBLISHED, got %v", stored.Status)
	}
	if stored.PublishedAt == nil {
		t.Error("expected non-nil PublishedAt")
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 || pub.published[0].ID != "msg-1" {
		t.Errorf("unexpected published messages: %v", pub.published)
	}
}

func TestOutbox_RetryAndDeadLetter(t *testing.T) {
	store := NewMemoryStore()
	pub := newMockPublisher()

	// Fail msg-2 indefinitely
	pub.failCount["msg-2"] = 999
	pub.failErr = errors.New("fatal broker error")

	proc := NewProcessor(store, pub, ProcessorConfig{
		PollInterval: 10 * time.Millisecond,
		InitialRetry: 5 * time.Millisecond,
		MaxRetry:     20 * time.Millisecond,
		BatchSize:    10,
		Concurrency:  2,
	})
	proc.Start()
	defer proc.Stop()

	ctx := context.Background()
	msg := Message{
		ID:          "msg-2",
		Topic:       "runs",
		MaxAttempts: 3,
	}

	if err := store.Stage(ctx, msg); err != nil {
		t.Fatalf("Stage failed: %v", err)
	}

	// Wait for message to exhaust max attempts and move to dead-letter
	deadline := time.Now().Add(2 * time.Second)
	var stored Message
	for time.Now().Before(deadline) {
		m, ok := store.Get("msg-2")
		if ok && m.Status == StatusDeadLetter {
			stored = m
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if stored.Status != StatusDeadLetter {
		t.Fatalf("expected status DEAD_LETTER, got %v (attempts: %d)", stored.Status, stored.Attempt)
	}
}

func TestOutbox_CleanPublished(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	now := time.Now().UTC()
	oldTime := now.Add(-2 * time.Hour)

	_ = store.Stage(ctx,
		Message{ID: "m1", Status: StatusPublished, PublishedAt: &oldTime},
		Message{ID: "m2", Status: StatusPublished, PublishedAt: &now},
		Message{ID: "m3", Status: StatusPending},
	)

	deleted, err := store.CleanPublished(ctx, now.Add(-1*time.Hour), 10)
	if err != nil {
		t.Fatalf("CleanPublished failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted message, got %d", deleted)
	}

	if _, ok := store.Get("m1"); ok {
		t.Error("expected m1 to be deleted")
	}
	if _, ok := store.Get("m2"); !ok {
		t.Error("expected m2 to remain")
	}
	if _, ok := store.Get("m3"); !ok {
		t.Error("expected m3 to remain")
	}
}

func TestOutbox_ConcurrentDispatch(t *testing.T) {
	store := NewMemoryStore()
	pub := newMockPublisher()

	proc := NewProcessor(store, pub, ProcessorConfig{
		PollInterval: 10 * time.Millisecond,
		BatchSize:    25,
		Concurrency:  8,
	})
	proc.Start()
	defer proc.Stop()

	ctx := context.Background()
	count := 50
	var msgs []Message
	for i := 0; i < count; i++ {
		msgs = append(msgs, Message{
			ID:    fmt.Sprintf("bulk-%d", i),
			Topic: "bulk-topic",
		})
	}

	if err := store.Stage(ctx, msgs...); err != nil {
		t.Fatalf("Stage failed: %v", err)
	}

	// Wait for all messages to be published
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pub.mu.Lock()
		pCount := len(pub.published)
		pub.mu.Unlock()
		if pCount >= count {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	pub.mu.Lock()
	pCount := len(pub.published)
	pub.mu.Unlock()

	if pCount != count {
		t.Fatalf("expected %d published, got %d", count, pCount)
	}
}

func TestOutbox_ProcessorLifecycle(t *testing.T) {
	store := NewMemoryStore()
	pub := newMockPublisher()

	proc := NewProcessor(store, pub, ProcessorConfig{
		PollInterval: 10 * time.Millisecond,
	})

	// Multiple Start / Stop calls should be safe
	proc.Start()
	proc.Start()

	time.Sleep(20 * time.Millisecond)

	proc.Stop()
	proc.Stop()
}

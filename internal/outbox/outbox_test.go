package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type testPayload struct {
	Value string `json:"value"`
}

func TestMemoryStore_SaveGet(t *testing.T) {
	ms := NewMemoryStore()
	msg := &Message{ID: "1", Type: "test", Status: StatusPending, Payload: json.RawMessage(`{"value":"test"}`)}
	if err := ms.Save(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	got, err := ms.Get(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "1" {
		t.Error("ID mismatch")
	}
}

func TestMemoryStore_ListPending(t *testing.T) {
	ms := NewMemoryStore()
	ms.Save(context.Background(), &Message{ID: "1", Type: "test", Status: StatusPending})
	ms.Save(context.Background(), &Message{ID: "2", Type: "test", Status: StatusSent})
	pending, err := ms.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending, got %d", len(pending))
	}
}

func TestMemoryStore_UpdateDelete(t *testing.T) {
	ms := NewMemoryStore()
	msg := &Message{ID: "1", Type: "test", Status: StatusPending}
	ms.Save(context.Background(), msg)
	msg.Status = StatusSent
	ms.Update(context.Background(), msg)
	got, _ := ms.Get(context.Background(), "1")
	if got.Status != StatusSent {
		t.Error("expected status sent")
	}
	ms.Delete(context.Background(), "1")
	_, err := ms.Get(context.Background(), "1")
	if err == nil {
		t.Error("expected not found")
	}
}

func TestProcessor_RegisterHandler(t *testing.T) {
	ms := NewMemoryStore()
	p := NewProcessor(ms, 100*time.Millisecond)
	var called atomic.Bool
	p.RegisterHandler("test", func(ctx context.Context, msg *Message) error {
		called.Store(true)
		return nil
	})
	msg := &Message{ID: "1", Type: "test", Status: StatusPending, Payload: json.RawMessage(`{"value":"test"}`)}
	ms.Save(context.Background(), msg)
	p.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	p.Stop()
	if !called.Load() {
		t.Error("expected handler called")
	}
}

func TestProcessor_Retry(t *testing.T) {
	ms := NewMemoryStore()
	p := NewProcessor(ms, 100*time.Millisecond)
	var attempts atomic.Int32
	p.RegisterHandler("test", func(ctx context.Context, msg *Message) error {
		attempts.Add(1)
		return fmt.Errorf("fail")
	})
	msg := &Message{ID: "1", Type: "test", Status: StatusPending, Payload: json.RawMessage(`{}`)}
	ms.Save(context.Background(), msg)
	p.Start(context.Background())
	time.Sleep(3 * time.Second)
	p.Stop()
	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts.Load())
	}
	got, _ := ms.Get(context.Background(), "1")
	if got.Status != StatusFailed && got.Status != StatusPending {
		t.Errorf("expected pending or failed, got %s", got.Status)
	}
}

func TestProcessor_Success(t *testing.T) {
	ms := NewMemoryStore()
	p := NewProcessor(ms, 50*time.Millisecond)
	var called atomic.Bool
	p.RegisterHandler("test", func(ctx context.Context, msg *Message) error {
		called.Store(true)
		return nil
	})
	msg := &Message{ID: "1", Type: "test", Status: StatusPending, Payload: json.RawMessage(`{}`)}
	ms.Save(context.Background(), msg)
	p.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	p.Stop()
	if !called.Load() {
		t.Error("expected handler called")
	}
	got, _ := ms.Get(context.Background(), "1")
	if got.Status != StatusSent {
		t.Errorf("expected sent, got %s", got.Status)
	}
}

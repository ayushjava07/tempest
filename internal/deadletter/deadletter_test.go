package deadletter

import (
	"context"
	"fmt"
	"testing"
)

type testPayload struct {
	Value string `json:"value"`
}

func TestQueue_EnqueueDequeue(t *testing.T) {
	q := NewQueue[testPayload](100)
	msg := &Message[testPayload]{ID: "1", Payload: testPayload{Value: "test"}}
	if err := q.Enqueue(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	dequeued, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dequeued.ID != "1" {
		t.Error("ID mismatch")
	}
	if dequeued.Payload.Value != "test" {
		t.Error("payload mismatch")
	}
}

func TestQueue_Len(t *testing.T) {
	q := NewQueue[testPayload](100)
	if q.Len() != 0 {
		t.Error("expected 0")
	}
	q.Enqueue(context.Background(), &Message[testPayload]{ID: "1", Payload: testPayload{Value: "a"}})
	if q.Len() != 1 {
		t.Error("expected 1")
	}
}

func TestQueue_Peek(t *testing.T) {
	q := NewQueue[testPayload](100)
	msg := &Message[testPayload]{ID: "1", Payload: testPayload{Value: "test"}}
	q.Enqueue(context.Background(), msg)
	peeked, err := q.Peek()
	if err != nil {
		t.Fatal(err)
	}
	if peeked.ID != "1" {
		t.Error("ID mismatch")
	}
	if q.Len() != 1 {
		t.Error("peek should not remove")
	}
}

func TestQueue_Full(t *testing.T) {
	q := NewQueue[testPayload](2)
	q.Enqueue(context.Background(), &Message[testPayload]{ID: "1", Payload: testPayload{Value: "a"}})
	q.Enqueue(context.Background(), &Message[testPayload]{ID: "2", Payload: testPayload{Value: "b"}})
	err := q.Enqueue(context.Background(), &Message[testPayload]{ID: "3", Payload: testPayload{Value: "c"}})
	if err == nil {
		t.Error("expected queue full error")
	}
}

func TestQueue_Clear(t *testing.T) {
	q := NewQueue[testPayload](100)
	q.Enqueue(context.Background(), &Message[testPayload]{ID: "1", Payload: testPayload{Value: "a"}})
	q.Clear()
	if q.Len() != 0 {
		t.Error("expected empty")
	}
}

func TestQueue_Retry(t *testing.T) {
	q := NewQueue[testPayload](100)
	msg := &Message[testPayload]{ID: "1", Payload: testPayload{Value: "test"}, Error: "first error", RetryCount: 1}
	q.Enqueue(context.Background(), msg)
	q.Retry(context.Background(), msg, fmt.Errorf("second error"))
	if msg.RetryCount != 2 {
		t.Errorf("expected retry count 2, got %d", msg.RetryCount)
	}
	if msg.Error != "second error" {
		t.Error("error not updated")
	}
}

func TestMessage_SerializeDeserialize(t *testing.T) {
	msg := &Message[testPayload]{
		ID:        "test-1",
		Payload:   testPayload{Value: "hello"},
		Headers:   map[string]string{"x": "y"},
		Error:     "timeout",
		RetryCount: 3,
	}
	data, err := msg.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Deserialize[testPayload](data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != "test-1" || restored.RetryCount != 3 {
		t.Error("deserialize mismatch")
	}
}

func TestQueue_Messages(t *testing.T) {
	q := NewQueue[testPayload](100)
	q.Enqueue(context.Background(), &Message[testPayload]{ID: "1", Payload: testPayload{Value: "a"}})
	q.Enqueue(context.Background(), &Message[testPayload]{ID: "2", Payload: testPayload{Value: "b"}})
	msgs := q.Messages()
	if len(msgs) != 2 {
		t.Errorf("expected 2, got %d", len(msgs))
	}
}
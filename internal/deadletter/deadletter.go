package deadletter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Message[T any] struct {
	ID        string         `json:"id"`
	Payload   T              `json:"payload"`
	Headers   map[string]string `json:"headers"`
	Error     string         `json:"error"`
	RetryCount int           `json:"retry_count"`
	CreatedAt time.Time      `json:"created_at"`
	FailedAt  time.Time      `json:"failed_at"`
}

type Queue[T any] struct {
	mu       sync.Mutex
	messages []*Message[T]
	maxSize  int
}

func NewQueue[T any](maxSize int) *Queue[T] {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Queue[T]{
		messages: make([]*Message[T], 0, maxSize),
		maxSize:  maxSize,
	}
}

func (q *Queue[T]) Enqueue(ctx context.Context, msg *Message[T]) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.messages) >= q.maxSize {
		return fmt.Errorf("queue full")
	}
	msg.FailedAt = time.Now()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	q.messages = append(q.messages, msg)
	return nil
}

func (q *Queue[T]) Dequeue(ctx context.Context) (*Message[T], error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.messages) == 0 {
		return nil, fmt.Errorf("empty")
	}
	msg := q.messages[0]
	q.messages = q.messages[1:]
	return msg, nil
}

func (q *Queue[T]) Peek() (*Message[T], error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.messages) == 0 {
		return nil, fmt.Errorf("empty")
	}
	return q.messages[0], nil
}

func (q *Queue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages)
}

func (q *Queue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = nil
}

func (q *Queue[T]) Messages() []*Message[T] {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*Message[T], len(q.messages))
	copy(result, q.messages)
	return result
}

func (q *Queue[T]) Retry(ctx context.Context, msg *Message[T], newErr error) error {
	msg.RetryCount++
	msg.Error = newErr.Error()
	return q.Enqueue(ctx, msg)
}

func (msg *Message[T]) Serialize() ([]byte, error) {
	return json.Marshal(msg)
}

func Deserialize[T any](data []byte) (*Message[T], error) {
	var msg Message[T]
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
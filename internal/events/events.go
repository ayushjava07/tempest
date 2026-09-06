package events

import (
	"context"
	"sync"
	"time"
)

type Handler func(ctx context.Context, event Event) error

type Event struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}

type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	queue    []Event
	maxQueue int
}

func NewBus(maxQueue int) *Bus {
	if maxQueue <= 0 {
		maxQueue = 1000
	}
	return &Bus{
		handlers: make(map[string][]Handler),
		maxQueue: maxQueue,
	}
}

func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	event.Timestamp = time.Now()
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.handlers["@all"] = append(b.handlers["@all"], b.handlers["@all"]...)
	allHandlers := b.handlers["@all"]
	b.mu.RUnlock()
	for _, h := range handlers {
		if err := h(ctx, event); err != nil {
			return err
		}
	}
	for _, h := range allHandlers {
		if err := h(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bus) Enqueue(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	event.Timestamp = time.Now()
	if len(b.queue) >= b.maxQueue {
		b.queue = b.queue[1:]
	}
	b.queue = append(b.queue, event)
}

func (b *Bus) Drain(ctx context.Context) int {
	b.mu.Lock()
	events := b.queue
	b.queue = nil
	b.mu.Unlock()
	count := 0
	for _, e := range events {
		_ = b.Publish(ctx, e)
		count++
	}
	return count
}

func (b *Bus) QueueSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.queue)
}

func (b *Bus) SubscriberCount(eventType string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.handlers[eventType])
}

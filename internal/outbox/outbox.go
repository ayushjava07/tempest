package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Message struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Headers   map[string]string `json:"headers"`
	CreatedAt time.Time      `json:"created_at"`
	RetryCount int           `json:"retry_count"`
	NextRetry time.Time      `json:"next_retry"`
	Status    string         `json:"status"`
}

const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusFailed  = "failed"
)

type Store interface {
	Save(ctx context.Context, msg *Message) error
	Get(ctx context.Context, id string) (*Message, error)
	ListPending(ctx context.Context, limit int) ([]*Message, error)
	Update(ctx context.Context, msg *Message) error
	Delete(ctx context.Context, id string) error
}

type MemoryStore struct {
	mu       sync.RWMutex
	messages map[string]*Message
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		messages: make(map[string]*Message),
	}
}

func (ms *MemoryStore) Save(ctx context.Context, msg *Message) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.messages[msg.ID] = msg
	return nil
}

func (ms *MemoryStore) Get(ctx context.Context, id string) (*Message, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	msg, ok := ms.messages[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return msg, nil
}

func (ms *MemoryStore) ListPending(ctx context.Context, limit int) ([]*Message, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	var pending []*Message
	for _, msg := range ms.messages {
		if msg.Status == StatusPending && (msg.NextRetry.IsZero() || time.Now().After(msg.NextRetry)) {
			pending = append(pending, msg)
			if len(pending) >= limit {
				break
			}
		}
	}
	return pending, nil
}

func (ms *MemoryStore) Update(ctx context.Context, msg *Message) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.messages[msg.ID] = msg
	return nil
}

func (ms *MemoryStore) Delete(ctx context.Context, id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.messages, id)
	return nil
}

type Processor struct {
	store    Store
	handlers map[string]func(ctx context.Context, msg *Message) error
	interval time.Duration
	stopCh   chan struct{}
}

func NewProcessor(store Store, interval time.Duration) *Processor {
	if interval <= 0 {
		interval = time.Second
	}
	return &Processor{
		store:    store,
		handlers: make(map[string]func(ctx context.Context, msg *Message) error),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (p *Processor) RegisterHandler(msgType string, handler func(ctx context.Context, msg *Message) error) {
	p.handlers[msgType] = handler
}

func (p *Processor) Start(ctx context.Context) {
	go p.run(ctx)
}

func (p *Processor) run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.processPending(ctx)
		}
	}
}

func (p *Processor) processPending(ctx context.Context) {
	msgs, err := p.store.ListPending(ctx, 100)
	if err != nil {
		return
	}
	for _, msg := range msgs {
		handler, ok := p.handlers[msg.Type]
		if !ok {
			msg.Status = StatusFailed
			msg.RetryCount++
			p.store.Update(ctx, msg)
			continue
		}
		if err := handler(ctx, msg); err != nil {
			msg.RetryCount++
			if msg.RetryCount >= 5 {
				msg.Status = StatusFailed
			} else {
				msg.Status = StatusPending
				msg.NextRetry = time.Now().Add(time.Duration(msg.RetryCount*msg.RetryCount) * time.Second)
			}
			p.store.Update(ctx, msg)
			continue
		}
		msg.Status = StatusSent
		p.store.Update(ctx, msg)
	}
}

func (p *Processor) Stop() {
	close(p.stopCh)
}
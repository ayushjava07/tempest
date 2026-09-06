package outbox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrStoreClosed         = errors.New("outbox: store is closed")
	ErrMessageNotFound     = errors.New("outbox: message not found")
	ErrMaxAttemptsExceeded = errors.New("outbox: max delivery attempts exceeded")
)

type MessageStatus string

const (
	StatusPending    MessageStatus = "PENDING"
	StatusInFlight   MessageStatus = "IN_FLIGHT"
	StatusPublished  MessageStatus = "PUBLISHED"
	StatusFailed     MessageStatus = "FAILED"
	StatusDeadLetter MessageStatus = "DEAD_LETTER"
)

// Message represents an outbox event staged for reliable downstream dispatch.
type Message struct {
	ID          string
	Topic       string
	Key         string
	Payload     []byte
	Headers     map[string]string
	CreatedAt   time.Time
	ScheduledAt time.Time
	PublishedAt *time.Time
	Attempt     int
	MaxAttempts int
	LastError   string
	Status      MessageStatus
}

// Publisher is responsible for delivering the message to the messaging broker.
type Publisher interface {
	Publish(ctx context.Context, msg Message) error
}

// Store persists outbox messages atomically alongside domain updates.
type Store interface {
	Stage(ctx context.Context, msgs ...Message) error
	FetchPending(ctx context.Context, batchSize int, now time.Time) ([]Message, error)
	MarkPublished(ctx context.Context, id string, at time.Time) error
	MarkFailed(ctx context.Context, id string, errStr string, nextRetry time.Time, attempt int) error
	MarkDeadLetter(ctx context.Context, id string, reason string) error
	CleanPublished(ctx context.Context, olderThan time.Time, limit int) (int, error)
}

// MemoryStore implements an in-memory thread-safe Store for testing and local usage.
type MemoryStore struct {
	mu       sync.RWMutex
	messages map[string]Message
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		messages: make(map[string]Message),
	}
}

func (s *MemoryStore) Stage(ctx context.Context, msgs ...Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range msgs {
		if m.ID == "" {
			return errors.New("outbox: message ID cannot be empty")
		}
		if m.Status == "" {
			m.Status = StatusPending
		}
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now().UTC()
		}
		if m.ScheduledAt.IsZero() {
			m.ScheduledAt = m.CreatedAt
		}
		if m.MaxAttempts <= 0 {
			m.MaxAttempts = 5
		}
		s.messages[m.ID] = m
	}
	return nil
}

func (s *MemoryStore) FetchPending(ctx context.Context, batchSize int, now time.Time) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Message
	for id, m := range s.messages {
		if (m.Status == StatusPending || m.Status == StatusFailed) && !m.ScheduledAt.After(now) {
			m.Status = StatusInFlight
			s.messages[id] = m
			out = append(out, m)
			if len(out) >= batchSize {
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) MarkPublished(ctx context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.messages[id]
	if !ok {
		return ErrMessageNotFound
	}
	m.Status = StatusPublished
	m.PublishedAt = &at
	s.messages[id] = m
	return nil
}

func (s *MemoryStore) MarkFailed(ctx context.Context, id string, errStr string, nextRetry time.Time, attempt int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.messages[id]
	if !ok {
		return ErrMessageNotFound
	}
	m.Attempt = attempt
	m.LastError = errStr
	m.ScheduledAt = nextRetry
	if m.Attempt >= m.MaxAttempts {
		m.Status = StatusDeadLetter
	} else {
		m.Status = StatusFailed
	}
	s.messages[id] = m
	return nil
}

func (s *MemoryStore) MarkDeadLetter(ctx context.Context, id string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.messages[id]
	if !ok {
		return ErrMessageNotFound
	}
	m.Status = StatusDeadLetter
	m.LastError = reason
	s.messages[id] = m
	return nil
}

func (s *MemoryStore) CleanPublished(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deleted := 0
	for id, m := range s.messages {
		if m.Status == StatusPublished && m.PublishedAt != nil && m.PublishedAt.Before(olderThan) {
			delete(s.messages, id)
			deleted++
			if limit > 0 && deleted >= limit {
				break
			}
		}
	}
	return deleted, nil
}

func (s *MemoryStore) Get(id string) (Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.messages[id]
	return m, ok
}

// ProcessorConfig contains tuning parameters for the outbox processor.
type ProcessorConfig struct {
	PollInterval time.Duration
	BatchSize    int
	Concurrency  int
	InitialRetry time.Duration
	MaxRetry     time.Duration
}

// Processor runs background polling and publishing of outbox messages.
type Processor struct {
	store     Store
	publisher Publisher
	cfg       ProcessorConfig
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   atomic.Bool
}

func NewProcessor(store Store, publisher Publisher, cfg ProcessorConfig) *Processor {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.InitialRetry <= 0 {
		cfg.InitialRetry = 200 * time.Millisecond
	}
	if cfg.MaxRetry <= 0 {
		cfg.MaxRetry = 1 * time.Minute
	}

	return &Processor{
		store:     store,
		publisher: publisher,
		cfg:       cfg,
		stopCh:    make(chan struct{}),
	}
}

func (p *Processor) Start() {
	if p.running.CompareAndSwap(false, true) {
		p.wg.Add(1)
		go p.pollLoop()
	}
}

func (p *Processor) Stop() {
	if p.running.CompareAndSwap(true, false) {
		close(p.stopCh)
		p.wg.Wait()
	}
}

func (p *Processor) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.dispatchBatch()
		}
	}
}

func (p *Processor) dispatchBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := p.store.FetchPending(ctx, p.cfg.BatchSize, time.Now().UTC())
	if err != nil || len(msgs) == 0 {
		return
	}

	workCh := make(chan Message, len(msgs))
	for _, m := range msgs {
		workCh <- m
	}
	close(workCh)

	var workerWg sync.WaitGroup
	workers := p.cfg.Concurrency
	if workers > len(msgs) {
		workers = len(msgs)
	}

	for i := 0; i < workers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for msg := range workCh {
				p.handleMessage(ctx, msg)
			}
		}()
	}
	workerWg.Wait()
}

func (p *Processor) handleMessage(ctx context.Context, msg Message) {
	err := p.publisher.Publish(ctx, msg)
	if err == nil {
		_ = p.store.MarkPublished(ctx, msg.ID, time.Now().UTC())
		return
	}

	// Calculate exponential backoff with full jitter
	attempt := msg.Attempt + 1
	if attempt >= msg.MaxAttempts {
		_ = p.store.MarkDeadLetter(ctx, msg.ID, fmt.Sprintf("failed after %d attempts: %v", attempt, err))
		return
	}

	backoff := float64(p.cfg.InitialRetry) * math.Pow(2, float64(attempt-1))
	if backoff > float64(p.cfg.MaxRetry) {
		backoff = float64(p.cfg.MaxRetry)
	}
	jitter := rand.Float64() * backoff * 0.2
	delay := time.Duration(backoff + jitter)

	nextRetry := time.Now().UTC().Add(delay)
	_ = p.store.MarkFailed(ctx, msg.ID, err.Error(), nextRetry, attempt)
}

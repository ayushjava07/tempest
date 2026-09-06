package deadletter

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrRecordNotFound = errors.New("dead letter record not found")
)

// DeadLetterRecord stores a failed run or task that exceeded its retry budget.
type DeadLetterRecord struct {
	ID         string
	RunID      string
	StepID     string
	Reason     string
	Attempts   int
	Payload    []byte
	CreatedAt  time.Time
	RedrivenAt *time.Time
}

// Queue manages dead-lettered executions in memory.
type Queue struct {
	mu      sync.RWMutex
	records map[string]*DeadLetterRecord
}

// New creates a new Dead Letter Queue.
func New() *Queue {
	return &Queue{
		records: make(map[string]*DeadLetterRecord),
	}
}

// Enqueue quarantines a failed task into the dead letter queue.
func (q *Queue) Enqueue(runID, stepID, reason string, attempts int, payload []byte) *DeadLetterRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	id := fmt.Sprintf("dlq-%s-%s-%d", runID, stepID, time.Now().UnixNano())
	rec := &DeadLetterRecord{
		ID:        id,
		RunID:     runID,
		StepID:    stepID,
		Reason:    reason,
		Attempts:  attempts,
		Payload:   payload,
		CreatedAt: time.Now(),
	}

	q.records[id] = rec
	return rec
}

// Get retrieves a dead letter record by ID.
func (q *Queue) Get(id string) (*DeadLetterRecord, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rec, exists := q.records[id]
	if !exists {
		return nil, ErrRecordNotFound
	}
	return rec, nil
}

// List returns all quarantined dead letter records.
func (q *Queue) List() []*DeadLetterRecord {
	q.mu.RLock()
	defer q.mu.RUnlock()

	list := make([]*DeadLetterRecord, 0, len(q.records))
	for _, rec := range q.records {
		list = append(list, rec)
	}
	return list
}

// MarkRedriven flags a dead letter record as redriven.
func (q *Queue) MarkRedriven(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	rec, exists := q.records[id]
	if !exists {
		return ErrRecordNotFound
	}

	now := time.Now()
	rec.RedrivenAt = &now
	return nil
}

// Purge removes all records older than maxAge.
func (q *Queue) Purge(maxAge time.Duration) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	purged := 0

	for id, rec := range q.records {
		if rec.CreatedAt.Before(cutoff) {
			delete(q.records, id)
			purged++
		}
	}
	return purged
}

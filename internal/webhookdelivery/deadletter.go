package webhookdelivery

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DeadLetterStore stores exhausted or failed webhook deliveries for manual inspection and replay.
type DeadLetterStore struct {
	mu      sync.RWMutex
	records map[string]DeliveryRecord
}

// NewDeadLetterStore creates a dead letter store.
func NewDeadLetterStore() *DeadLetterStore {
	return &DeadLetterStore{
		records: make(map[string]DeliveryRecord),
	}
}

// Save stores or updates a dead-letter delivery record.
func (s *DeadLetterStore) Save(record DeliveryRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.Status = StatusDeadLetter
	s.records[record.ID] = record
}

// Get retrieves a dead-letter record by delivery ID.
func (s *DeadLetterStore) Get(id string) (DeliveryRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	return rec, ok
}

// List returns dead-letter records, optionally filtered by endpointID.
func (s *DeadLetterStore) List(endpointID string) []DeliveryRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []DeliveryRecord
	for _, rec := range s.records {
		if endpointID == "" || rec.EndpointID == endpointID {
			result = append(result, rec)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Redeliver resets delivery status and attempts redelivery through worker.
func (s *DeadLetterStore) Redeliver(ctx context.Context, id string, worker *DeliveryWorker, ep EndpointConfig) error {
	s.mu.Lock()
	rec, exists := s.records[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("dead-letter record %s not found", id)
	}
	s.mu.Unlock()

	// Reset status for manual redelivery attempt
	rec.Status = StatusPending

	err := worker.Deliver(ctx, &rec, ep, SignPayload)
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec.Status == StatusSuccess {
		delete(s.records, id)
		return nil
	}

	// Update record with newly attempted attempt
	rec.Status = StatusDeadLetter
	s.records[id] = rec
	return err
}

// Purge removes records older than the given cutoff timestamp.
func (s *DeadLetterStore) Purge(cutoff time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	purged := 0
	for id, rec := range s.records {
		if rec.CreatedAt.Before(cutoff) {
			delete(s.records, id)
			purged++
		}
	}
	return purged
}

// Count returns the number of dead-letter records.
func (s *DeadLetterStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

package leader

import (
	"sync"
	"time"
)

// InMemCoordinator provides an in-memory lease coordinator with fencing terms.
type InMemCoordinator struct {
	mu     sync.Mutex
	record Record
}

// NewInMemCoordinator creates an in-memory election coordinator.
func NewInMemCoordinator() *InMemCoordinator {
	return &InMemCoordinator{}
}

// TryAcquireOrRenew checks if the lease is free or renewable by the candidate.
func (c *InMemCoordinator) TryAcquireOrRenew(candidateID string, term uint64, ttl time.Duration) (Record, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()

	// 1. If currently held by someone else and not expired
	if c.record.LeaderID != "" && c.record.LeaderID != candidateID && now.Before(c.record.ExpiresAt) {
		return c.record, false, nil
	}

	// 2. Either lease is expired, unowned, or currently held by this candidate
	if c.record.LeaderID != candidateID || now.After(c.record.ExpiresAt) {
		// New term when taking over
		c.record.Term++
	}

	c.record.LeaderID = candidateID
	c.record.ExpiresAt = now.Add(ttl)

	return c.record, true, nil
}

// GetLeader returns the active leader record.
func (c *InMemCoordinator) GetLeader() (Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().UTC().After(c.record.ExpiresAt) {
		return Record{}, ErrNotLeader
	}
	return c.record, nil
}

// Release yields the lease if held by candidateID.
func (c *InMemCoordinator) Release(candidateID string, term uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.record.LeaderID == candidateID {
		c.record.LeaderID = ""
		c.record.ExpiresAt = time.Time{}
	}
	return nil
}

package fencing

import (
	"fmt"
	"sync"
	"time"
)

// LeaseManager coordinates heartbeats, lease extensions, and preemption checks for active fencing tokens.
type LeaseManager struct {
	mu          sync.RWMutex
	generator   *TokenGenerator
	preemptSubs map[string][]func(FencingToken)
}

// NewLeaseManager creates a lease manager bound to a token generator.
func NewLeaseManager(gen *TokenGenerator) *LeaseManager {
	return &LeaseManager{
		generator:   gen,
		preemptSubs: make(map[string][]func(FencingToken)),
	}
}

// CheckPreemption verifies that the token is still the latest issued token and has not expired.
func (m *LeaseManager) CheckPreemption(current FencingToken) error {
	if current.IsExpired() {
		return fmt.Errorf("%w: token %d expired at %s", ErrTokenExpired, current.Token, current.ExpiresAt)
	}

	latest, ok := m.generator.LatestToken(current.Resource)
	if !ok {
		return nil
	}

	if latest.Token > current.Token {
		return fmt.Errorf("%w: resource %s preempted by token %d (current=%d)",
			ErrPreempted, current.Resource, latest.Token, current.Token)
	}

	return nil
}

// ExtendLease extends the expiration deadline of an active token if not preempted.
func (m *LeaseManager) ExtendLease(current FencingToken, additionalTTL time.Duration) (FencingToken, error) {
	if err := m.CheckPreemption(current); err != nil {
		return current, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check under lock
	latest, ok := m.generator.LatestToken(current.Resource)
	if ok && latest.Token > current.Token {
		return current, fmt.Errorf("%w: cannot extend preempted token %d < %d",
			ErrPreempted, current.Token, latest.Token)
	}

	if additionalTTL <= 0 {
		additionalTTL = 30 * time.Second
	}

	newExpiry := time.Now().UTC().Add(additionalTTL)
	updated := FencingToken{
		Resource:  current.Resource,
		Token:     current.Token,
		Owner:     current.Owner,
		Epoch:     current.Epoch,
		IssuedAt:  current.IssuedAt,
		ExpiresAt: newExpiry,
	}

	m.generator.mu.Lock()
	m.generator.activeTokens[current.Resource] = updated
	m.generator.mu.Unlock()

	return updated, nil
}

// NotifyPreemption triggers registered callbacks when a higher token takes over.
func (m *LeaseManager) NotifyPreemption(newToken FencingToken) {
	m.mu.RLock()
	callbacks, exists := m.preemptSubs[newToken.Resource]
	m.mu.RUnlock()

	if exists {
		for _, cb := range callbacks {
			cb(newToken)
		}
	}
}

// OnPreemption registers a hook called if another worker supersedes this resource.
func (m *LeaseManager) OnPreemption(resource string, callback func(FencingToken)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preemptSubs[resource] = append(m.preemptSubs[resource], callback)
}

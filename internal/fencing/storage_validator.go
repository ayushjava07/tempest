package fencing

import (
	"fmt"
	"sync"
)

// StorageEntry represents stored data guarded by a fencing token sequence.
type StorageEntry struct {
	Value        []byte       `json:"value"`
	FencingToken FencingToken `json:"fencing_token"`
}

// FencedStorage validates fencing tokens on every mutation to reject stale zombie writes.
type FencedStorage struct {
	mu           sync.RWMutex
	store        map[string]StorageEntry
	highestToken map[string]uint64
	rejectedCnt  uint64
}

// NewFencedStorage creates a storage layer guarded by fencing token validation.
func NewFencedStorage() *FencedStorage {
	return &FencedStorage{
		store:        make(map[string]StorageEntry),
		highestToken: make(map[string]uint64),
	}
}

// Put writes a key-value pair only if the provided token is strictly >= highest seen token.
func (s *FencedStorage) Put(key string, value []byte, token FencingToken) error {
	if token.IsExpired() {
		return fmt.Errorf("%w: token %d for %s expired at %s",
			ErrTokenExpired, token.Token, key, token.ExpiresAt)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	highest, exists := s.highestToken[key]
	if exists && token.Token < highest {
		s.rejectedCnt++
		return fmt.Errorf("%w: rejected token %d < highest observed token %d for key %q",
			ErrStaleFencingToken, token.Token, highest, key)
	}

	// Update highest seen token and persist entry
	s.highestToken[key] = token.Token
	s.store[key] = StorageEntry{
		Value:        append([]byte(nil), value...),
		FencingToken: token,
	}
	return nil
}

// Get reads the current value and the fencing token that authored it.
func (s *FencedStorage) Get(key string) ([]byte, FencingToken, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.store[key]
	if !ok {
		return nil, FencingToken{}, false
	}
	return append([]byte(nil), entry.Value...), entry.FencingToken, true
}

// Delete removes an entry only if the token is valid and not superseded.
func (s *FencedStorage) Delete(key string, token FencingToken) error {
	if token.IsExpired() {
		return ErrTokenExpired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	highest, exists := s.highestToken[key]
	if exists && token.Token < highest {
		s.rejectedCnt++
		return fmt.Errorf("%w: cannot delete with stale token %d < highest %d",
			ErrStaleFencingToken, token.Token, highest)
	}

	s.highestToken[key] = token.Token
	delete(s.store, key)
	return nil
}

// HighestObservedToken returns the maximum token sequence processed for a key.
func (s *FencedStorage) HighestObservedToken(key string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.highestToken[key]
}

// RejectedCount returns total rejected zombie writes.
func (s *FencedStorage) RejectedCount() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rejectedCnt
}

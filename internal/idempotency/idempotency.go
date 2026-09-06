package idempotency

import (
	"sync"
	"time"
)

type Key struct {
	ID        string
	Namespace string
}

type Entry struct {
	Key       Key
	Result    any
	Error     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store struct {
	mu      sync.RWMutex
	entries map[Key]*Entry
	ttl     time.Duration
}

func NewStore(ttl time.Duration) *Store {
	s := &Store{
		entries: make(map[Key]*Entry),
		ttl:     ttl,
	}
	go s.cleanup()
	return s
}

func (s *Store) Check(id, namespace string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := Key{ID: id, Namespace: namespace}
	e, ok := s.entries[k]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.ExpiresAt) {
		return nil, false
	}
	return e, true
}

func (s *Store) Mark(id, namespace string, result any, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := Key{ID: id, Namespace: namespace}
	s.entries[k] = &Entry{
		Key:       k,
		Result:    result,
		Error:     errMsg,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(s.ttl),
	}
}

func (s *Store) Remove(id, namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := Key{ID: id, Namespace: namespace}
	delete(s.entries, k)
}

func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *Store) cleanup() {
	ticker := time.NewTicker(s.ttl / 2)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, e := range s.entries {
			if now.After(e.ExpiresAt) {
				delete(s.entries, k)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[Key]*Entry)
}

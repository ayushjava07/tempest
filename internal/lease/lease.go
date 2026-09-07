package lease

import (
	"context"
	"sync"
	"time"
)

type Lease struct {
	ID        string
	Holder    string
	Resource  string
	ExpiresAt time.Time
}

type Manager struct {
	mu      sync.RWMutex
	leases  map[string]*Lease
	tokenFn func() string
}

func NewManager(tokenFn func() string) *Manager {
	if tokenFn == nil {
		tokenFn = func() string { return "" }
	}
	return &Manager{
		leases:  make(map[string]*Lease),
		tokenFn: tokenFn,
	}
}

func (m *Manager) Acquire(ctx context.Context, holder, resource string, ttl time.Duration) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.leases[resource]; ok {
		if time.Now().Before(existing.ExpiresAt) && existing.Holder != holder {
			return nil, &LeaseConflictError{Resource: resource, Holder: existing.Holder}
		}
	}
	lease := &Lease{
		ID:        m.tokenFn(),
		Holder:    holder,
		Resource:  resource,
		ExpiresAt: time.Now().Add(ttl),
	}
	m.leases[resource] = lease
	return lease, nil
}

func (m *Manager) Renew(ctx context.Context, holder, resource string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.leases[resource]
	if !ok {
		return &LeaseNotFoundError{Resource: resource}
	}
	if lease.Holder != holder {
		return &LeaseConflictError{Resource: resource, Holder: lease.Holder}
	}
	lease.ExpiresAt = time.Now().Add(ttl)
	return nil
}

func (m *Manager) Release(ctx context.Context, holder, resource string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.leases[resource]
	if !ok {
		return &LeaseNotFoundError{Resource: resource}
	}
	if lease.Holder != holder {
		return &LeaseConflictError{Resource: resource, Holder: lease.Holder}
	}
	delete(m.leases, resource)
	return nil
}

func (m *Manager) Get(resource string) (*Lease, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lease, ok := m.leases[resource]
	if !ok {
		return nil, false
	}
	if time.Now().After(lease.ExpiresAt) {
		return nil, false
	}
	return lease, true
}

func (m *Manager) Expired() []*Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var expired []*Lease
	for resource, lease := range m.leases {
		if now.After(lease.ExpiresAt) {
			expired = append(expired, lease)
			delete(m.leases, resource)
		}
	}
	return expired
}

func (m *Manager) All() []*Lease {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Lease
	for _, lease := range m.leases {
		if time.Now().Before(lease.ExpiresAt) {
			result = append(result, lease)
		}
	}
	return result
}

type LeaseNotFoundError struct {
	Resource string
}

func (e *LeaseNotFoundError) Error() string {
	return "lease not found: " + e.Resource
}

type LeaseConflictError struct {
	Resource string
	Holder   string
}

func (e *LeaseConflictError) Error() string {
	return "lease conflict: " + e.Resource + " held by " + e.Holder
}
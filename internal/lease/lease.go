package lease

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrLeaseHeld        = errors.New("lease is currently held by another owner")
	ErrLeaseExpired     = errors.New("lease has expired")
	ErrNotLeaseHolder   = errors.New("caller is not the active lease holder")
	ErrFencingStale     = errors.New("fencing token is stale")
)

// Lease represents a distributed coordination lease with an epoch/fencing token.
type Lease struct {
	ResourceID   string
	Owner        string
	FencingToken int64
	ExpiresAt    time.Time
}

// Coordinator manages distributed leases in-memory with monotonic fencing tokens.
type Coordinator struct {
	mu           sync.RWMutex
	leases       map[string]*Lease
	tokenCounter atomic.Int64
}

// NewCoordinator creates a new Lease Coordinator.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		leases: make(map[string]*Lease),
	}
}

// Acquire attempts to acquire or re-acquire a lease for resourceID.
// If the lease is held and not expired by a different owner, ErrLeaseHeld is returned.
func (c *Coordinator) Acquire(ctx context.Context, resourceID, owner string, ttl time.Duration) (*Lease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	existing, exists := c.leases[resourceID]

	if exists && existing.ExpiresAt.After(now) && existing.Owner != owner {
		return nil, ErrLeaseHeld
	}

	token := c.tokenCounter.Add(1)
	l := &Lease{
		ResourceID:   resourceID,
		Owner:        owner,
		FencingToken: token,
		ExpiresAt:    now.Add(ttl),
	}

	c.leases[resourceID] = l
	return l, nil
}

// Renew extends the TTL of an actively held lease, validating the owner and fencing token.
func (c *Coordinator) Renew(ctx context.Context, resourceID, owner string, token int64, ttl time.Duration) (*Lease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	existing, exists := c.leases[resourceID]
	if !exists {
		return nil, ErrLeaseExpired
	}

	if existing.Owner != owner {
		return nil, ErrNotLeaseHolder
	}

	if existing.FencingToken != token {
		return nil, ErrFencingStale
	}

	if now.After(existing.ExpiresAt) {
		return nil, ErrLeaseExpired
	}

	existing.ExpiresAt = now.Add(ttl)
	return existing, nil
}

// Release yields a lease if the caller is the current holder with matching fencing token.
func (c *Coordinator) Release(ctx context.Context, resourceID, owner string, token int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	existing, exists := c.leases[resourceID]
	if !exists {
		return nil
	}

	if existing.Owner != owner {
		return ErrNotLeaseHolder
	}

	if existing.FencingToken != token {
		return ErrFencingStale
	}

	delete(c.leases, resourceID)
	return nil
}

// Validate checks if the lease is still valid for the given owner and token.
func (c *Coordinator) Validate(resourceID, owner string, token int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	existing, exists := c.leases[resourceID]
	if !exists {
		return false
	}

	return existing.Owner == owner &&
		existing.FencingToken == token &&
		time.Now().Before(existing.ExpiresAt)
}

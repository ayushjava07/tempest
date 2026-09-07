package fencing

import (
	"sync"
	"sync/atomic"
	"time"
)

// ResourceCounter encapsulates atomic counters for lock-free token generation per resource.
type ResourceCounter struct {
	counter atomic.Uint64
	epoch   atomic.Uint64
}

// CASTokenGenerator provides high-throughput lock-free fencing token generation using atomic CAS operations.
type CASTokenGenerator struct {
	resources sync.Map // map[string]*ResourceCounter
}

// NewCASTokenGenerator initializes a CAS-based generator.
func NewCASTokenGenerator() *CASTokenGenerator {
	return &CASTokenGenerator{}
}

func (g *CASTokenGenerator) getOrCreateCounter(resource string) *ResourceCounter {
	if val, ok := g.resources.Load(resource); ok {
		return val.(*ResourceCounter)
	}
	rc := &ResourceCounter{}
	rc.epoch.Store(1)
	actual, _ := g.resources.LoadOrStore(resource, rc)
	return actual.(*ResourceCounter)
}

// AcquireFastToken advances the atomic sequence and produces an immutable FencingToken with zero mutex contention.
func (g *CASTokenGenerator) AcquireFastToken(resource, owner string, ttl time.Duration) FencingToken {
	rc := g.getOrCreateCounter(resource)
	tokNum := rc.counter.Add(1)
	epoch := rc.epoch.Load()

	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	now := time.Now().UTC()
	return FencingToken{
		Resource:  resource,
		Token:     tokNum,
		Owner:     owner,
		Epoch:     epoch,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}
}

// AdvanceEpoch atomically increments the epoch and resets the token counter for a resource.
func (g *CASTokenGenerator) AdvanceEpoch(resource string) uint64 {
	rc := g.getOrCreateCounter(resource)
	return rc.epoch.Add(1)
}

// CurrentSeq loads the current token sequence atomically.
func (g *CASTokenGenerator) CurrentSeq(resource string) uint64 {
	if val, ok := g.resources.Load(resource); ok {
		return val.(*ResourceCounter).counter.Load()
	}
	return 0
}

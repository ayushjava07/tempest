package loadbalancer

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
)

var (
	ErrNoHealthyTargets = errors.New("loadbalancer: no healthy targets available")
	ErrTargetNotFound   = errors.New("loadbalancer: target not found")
	ErrDuplicateTarget  = errors.New("loadbalancer: target already exists")
)

// Strategy specifies the target selection algorithm.
type Strategy int

const (
	StrategyRoundRobin Strategy = iota
	StrategyWeightedRoundRobin
	StrategyLeastConnections
	StrategyPowerOfTwoChoices
	StrategyConsistentHash
)

// Target represents a backend worker or service instance.
type Target struct {
	ID            string
	Address       string
	Weight        int
	Healthy       bool
	ActiveConns   atomic.Int64
	TotalRequests atomic.Int64

	// Internal state for smooth weighted round robin
	currentWeight int
}

type vnode struct {
	hash     uint64
	targetID string
}

// Balancer distributes requests across healthy targets.
type Balancer struct {
	mu           sync.RWMutex
	strategy     Strategy
	targets      map[string]*Target
	targetList   []*Target
	healthyList  []*Target
	rrIndex      atomic.Uint64
	vnodesPerTgt int
	ring         []vnode
}

// New creates a new Balancer with the given strategy.
func New(strategy Strategy) *Balancer {
	return &Balancer{
		strategy:     strategy,
		targets:      make(map[string]*Target),
		vnodesPerTgt: 50,
	}
}

// AddTarget registers a new target.
func (b *Balancer) AddTarget(id, address string, weight int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.targets[id]; exists {
		return ErrDuplicateTarget
	}

	if weight <= 0 {
		weight = 1
	}

	t := &Target{
		ID:      id,
		Address: address,
		Weight:  weight,
		Healthy: true,
	}

	b.targets[id] = t
	b.rebuildListsLocked()
	return nil
}

// RemoveTarget removes a target.
func (b *Balancer) RemoveTarget(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.targets[id]; !exists {
		return ErrTargetNotFound
	}

	delete(b.targets, id)
	b.rebuildListsLocked()
	return nil
}

// SetHealth updates the health status of a target.
func (b *Balancer) SetHealth(id string, healthy bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	t, exists := b.targets[id]
	if !exists {
		return ErrTargetNotFound
	}

	t.Healthy = healthy
	b.rebuildListsLocked()
	return nil
}

func (b *Balancer) rebuildListsLocked() {
	b.targetList = make([]*Target, 0, len(b.targets))
	b.healthyList = make([]*Target, 0, len(b.targets))

	for _, t := range b.targets {
		b.targetList = append(b.targetList, t)
		if t.Healthy {
			b.healthyList = append(b.healthyList, t)
		}
	}

	// Sort targets deterministically by ID
	sort.Slice(b.healthyList, func(i, j int) bool {
		return b.healthyList[i].ID < b.healthyList[j].ID
	})

	// Rebuild consistent hash ring if needed
	b.ring = nil
	if b.strategy == StrategyConsistentHash {
		for _, t := range b.healthyList {
			for i := 0; i < b.vnodesPerTgt*t.Weight; i++ {
				h := hashKey(fmt.Sprintf("%s#%d", t.ID, i))
				b.ring = append(b.ring, vnode{hash: h, targetID: t.ID})
			}
		}
		sort.Slice(b.ring, func(i, j int) bool {
			return b.ring[i].hash < b.ring[j].hash
		})
	}
}

func hashKey(key string) uint64 {
	sum := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}

// Select picks an optimal target according to the configured strategy.
func (b *Balancer) Select(key string) (*Target, func(), error) {
	b.mu.Lock()
	if len(b.healthyList) == 0 {
		b.mu.Unlock()
		return nil, nil, ErrNoHealthyTargets
	}

	var chosen *Target

	switch b.strategy {
	case StrategyRoundRobin:
		idx := b.rrIndex.Add(1) - 1
		chosen = b.healthyList[int(idx%uint64(len(b.healthyList)))]

	case StrategyWeightedRoundRobin:
		// Nginx smooth weighted round-robin algorithm
		totalWeight := 0
		var best *Target
		for _, t := range b.healthyList {
			t.currentWeight += t.Weight
			totalWeight += t.Weight
			if best == nil || t.currentWeight > best.currentWeight {
				best = t
			}
		}
		if best != nil {
			best.currentWeight -= totalWeight
		}
		chosen = best

	case StrategyLeastConnections:
		minConns := int64(1<<62 - 1)
		for _, t := range b.healthyList {
			conns := t.ActiveConns.Load()
			if conns < minConns {
				minConns = conns
				chosen = t
			}
		}

	case StrategyPowerOfTwoChoices:
		n := len(b.healthyList)
		if n == 1 {
			chosen = b.healthyList[0]
		} else {
			i := rand.Intn(n)
			j := rand.Intn(n - 1)
			if j >= i {
				j++
			}
			t1 := b.healthyList[i]
			t2 := b.healthyList[j]
			if t1.ActiveConns.Load() <= t2.ActiveConns.Load() {
				chosen = t1
			} else {
				chosen = t2
			}
		}

	case StrategyConsistentHash:
		if len(b.ring) == 0 {
			b.mu.Unlock()
			return nil, nil, ErrNoHealthyTargets
		}
		h := hashKey(key)
		idx := sort.Search(len(b.ring), func(i int) bool {
			return b.ring[i].hash >= h
		})
		if idx == len(b.ring) {
			idx = 0
		}
		targetID := b.ring[idx].targetID
		chosen = b.targets[targetID]
	}

	chosen.ActiveConns.Add(1)
	chosen.TotalRequests.Add(1)
	b.mu.Unlock()

	done := func() {
		chosen.ActiveConns.Add(-1)
	}

	return chosen, done, nil
}

// GetTarget returns a copy of the target status.
func (b *Balancer) GetTarget(id string) (*Target, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	t, ok := b.targets[id]
	if !ok {
		return nil, ErrTargetNotFound
	}

	return &Target{
		ID:      t.ID,
		Address: t.Address,
		Weight:  t.Weight,
		Healthy: t.Healthy,
	}, nil
}

package barrier

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrBarrierBroken    = errors.New("barrier: barrier broken")
	ErrBarrierTimeout   = errors.New("barrier: timeout waiting for threshold")
	ErrAlreadyArrived   = errors.New("barrier: party already arrived in current generation")
	ErrInvalidThreshold = errors.New("barrier: threshold must be > 0")
)

// Barrier coordinates synchronization of multiple parties.
type Barrier struct {
	mu         sync.Mutex
	cond       *sync.Cond
	threshold  int
	arrived    map[string]time.Time
	generation uint64
	broken     bool
	brokenErr  error
}

// NewBarrier creates a barrier that trips when `threshold` parties arrive.
func NewBarrier(threshold int) (*Barrier, error) {
	if threshold <= 0 {
		return nil, ErrInvalidThreshold
	}

	b := &Barrier{
		threshold: threshold,
		arrived:   make(map[string]time.Time),
	}
	b.cond = sync.NewCond(&b.mu)
	return b, nil
}

// ArriveAndWait registers arrival and blocks until threshold is reached or context expires.
// Returns (isLast, error).
func (b *Barrier) ArriveAndWait(ctx context.Context, partyID string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.broken {
		return false, b.brokenErr
	}

	gen := b.generation

	if _, exists := b.arrived[partyID]; exists {
		return false, ErrAlreadyArrived
	}

	b.arrived[partyID] = time.Now().UTC()

	// Check if this arrival trips the barrier
	if len(b.arrived) >= b.threshold {
		// Tripped! Advance generation and release all waiting parties
		b.generation++
		b.arrived = make(map[string]time.Time)
		b.cond.Broadcast()
		return true, nil
	}

	// Not tripped yet: wait for current generation to complete
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			b.mu.Lock()
			if gen == b.generation && !b.broken {
				b.breakLocked(ctx.Err())
			}
			b.mu.Unlock()
		case <-done:
		}
	}()

	for gen == b.generation && !b.broken {
		b.cond.Wait()
	}
	close(done)

	if b.broken {
		return false, b.brokenErr
	}

	return false, nil
}

// Break manually breaks the barrier, waking all waiting parties with err.
func (b *Barrier) Break(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.breakLocked(err)
}

func (b *Barrier) breakLocked(err error) {
	if b.broken {
		return
	}
	b.broken = true
	if err == nil {
		b.brokenErr = ErrBarrierBroken
	} else {
		b.brokenErr = err
	}
	b.cond.Broadcast()
}

// Reset clears the broken state and advances to a fresh generation.
func (b *Barrier) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.broken = false
	b.brokenErr = nil
	b.generation++
	b.arrived = make(map[string]time.Time)
	b.cond.Broadcast()
}

// ArrivedCount returns number of parties waiting in the current generation.
func (b *Barrier) ArrivedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.arrived)
}

// Coordinator manages multiple named rendezvous barriers across workflows.
type Coordinator struct {
	mu       sync.RWMutex
	barriers map[string]*Barrier
}

// NewCoordinator creates a named barrier manager.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		barriers: make(map[string]*Barrier),
	}
}

// GetOrCreate returns or creates a named barrier.
func (c *Coordinator) GetOrCreate(name string, threshold int) (*Barrier, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.barriers[name]
	if !ok {
		var err error
		b, err = NewBarrier(threshold)
		if err != nil {
			return nil, err
		}
		c.barriers[name] = b
	}
	return b, nil
}

// Remove deletes a named barrier.
func (c *Coordinator) Remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if b, ok := c.barriers[name]; ok {
		b.Break(ErrBarrierBroken)
		delete(c.barriers, name)
	}
}

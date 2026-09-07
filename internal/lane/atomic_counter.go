package lane

import (
	"sync/atomic"
)

// AtomicQuantumCounter maintains atomic credits and deficit tracking for high-frequency scheduler fast paths.
type AtomicQuantumCounter struct {
	credits atomic.Int64
	depth   atomic.Int64
}

// NewAtomicQuantumCounter creates an atomic quantum counter.
func NewAtomicQuantumCounter(initialCredits int64) *AtomicQuantumCounter {
	c := &AtomicQuantumCounter{}
	c.credits.Store(initialCredits)
	return c
}

// Add atomically increases credit by delta.
func (c *AtomicQuantumCounter) Add(delta int64) int64 {
	return c.credits.Add(delta)
}

// Sub atomically decreases credit by delta.
func (c *AtomicQuantumCounter) Sub(delta int64) int64 {
	return c.credits.Add(-delta)
}

// Load returns current credit balance.
func (c *AtomicQuantumCounter) Load() int64 {
	return c.credits.Load()
}

// Reset resets credit balance to zero.
func (c *AtomicQuantumCounter) Reset() {
	c.credits.Store(0)
}

// TryConsume attempts to atomically deduct cost if balance >= cost.
func (c *AtomicQuantumCounter) TryConsume(cost int64) bool {
	for {
		curr := c.credits.Load()
		if curr < cost {
			return false
		}
		if c.credits.CompareAndSwap(curr, curr-cost) {
			return true
		}
	}
}

// IncDepth increments task count.
func (c *AtomicQuantumCounter) IncDepth() int64 {
	return c.depth.Add(1)
}

// DecDepth decrements task count.
func (c *AtomicQuantumCounter) DecDepth() int64 {
	return c.depth.Add(-1)
}

// Depth returns current task count.
func (c *AtomicQuantumCounter) Depth() int64 {
	return c.depth.Load()
}

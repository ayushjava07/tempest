package throttler

import (
	"sync"
	"time"
)

// CompressedTimestampRing maintains a fixed-capacity circular ring of relative millisecond timestamps
// to eliminate heap allocation during high-frequency sliding-window evaluations.
type CompressedTimestampRing struct {
	mu       sync.RWMutex
	baseTime time.Time
	ring     []int32 // millisecond offsets relative to baseTime
	head     int
	tail     int
	size     int
	capacity int
}

// NewCompressedTimestampRing creates a zero-allocation circular buffer.
func NewCompressedTimestampRing(capacity int) *CompressedTimestampRing {
	if capacity <= 0 {
		capacity = 1024
	}
	return &CompressedTimestampRing{
		baseTime: time.Now(),
		ring:     make([]int32, capacity),
		capacity: capacity,
	}
}

// Push records a timestamp into the circular ring.
func (r *CompressedTimestampRing) Push(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	offset := int32(t.Sub(r.baseTime).Milliseconds())

	if r.size == r.capacity {
		// Overwrite oldest
		r.tail = (r.tail + 1) % r.capacity
		r.size--
	}

	r.ring[r.head] = offset
	r.head = (r.head + 1) % r.capacity
	r.size++
}

// CountWithinWindow counts how many recorded timestamps fall within now - window.
func (r *CompressedTimestampRing) CountWithinWindow(now time.Time, window time.Duration) (count int, oldestOffset time.Duration) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return 0, 0
	}

	nowOffset := int32(now.Sub(r.baseTime).Milliseconds())
	cutoffOffset := nowOffset - int32(window.Milliseconds())

	idx := r.tail
	validCount := 0
	foundOldest := false
	var oldestVal int32

	for i := 0; i < r.size; i++ {
		val := r.ring[idx]
		if val >= cutoffOffset {
			validCount++
			if !foundOldest {
				oldestVal = val
				foundOldest = true
			}
		}
		idx = (idx + 1) % r.capacity
	}

	if foundOldest {
		oldestOffset = time.Duration(nowOffset-oldestVal) * time.Millisecond
	}

	return validCount, oldestOffset
}

// Len returns current count of entries in the ring.
func (r *CompressedTimestampRing) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

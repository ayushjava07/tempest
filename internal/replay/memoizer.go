package replay

import (
	"sort"
	"sync"
)

// Checkpoint represents a memoized point-in-time workflow state.
type Checkpoint struct {
	Seq   uint64            `json:"seq"`
	State map[string]string `json:"state"`
}

// StreamMemoizer caches intermediate execution states at sequence boundaries to accelerate long-trace replay.
type StreamMemoizer struct {
	mu          sync.RWMutex
	interval    uint64
	maxEntries  int
	checkpoints map[uint64]map[string]string
	sortedSeqs  []uint64
}

// NewStreamMemoizer creates a memoizer with a snapshot interval and capacity limit.
func NewStreamMemoizer(interval uint64, maxEntries int) *StreamMemoizer {
	if interval == 0 {
		interval = 100
	}
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &StreamMemoizer{
		interval:    interval,
		maxEntries:  maxEntries,
		checkpoints: make(map[uint64]map[string]string),
		sortedSeqs:  make([]uint64, 0),
	}
}

// MaybeMemoize saves a state snapshot if seq lands on an interval boundary or when explicitly requested.
func (m *StreamMemoizer) MaybeMemoize(seq uint64, state map[string]string, force bool) bool {
	if !force && (seq%m.interval != 0) {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Evict oldest checkpoint if capacity reached
	if len(m.checkpoints) >= m.maxEntries {
		if _, exists := m.checkpoints[seq]; !exists && len(m.sortedSeqs) > 0 {
			oldest := m.sortedSeqs[0]
			delete(m.checkpoints, oldest)
			m.sortedSeqs = m.sortedSeqs[1:]
		}
	}

	// Deep copy state
	cp := make(map[string]string, len(state))
	for k, v := range state {
		cp[k] = v
	}

	if _, exists := m.checkpoints[seq]; !exists {
		m.sortedSeqs = append(m.sortedSeqs, seq)
		sort.Slice(m.sortedSeqs, func(i, j int) bool { return m.sortedSeqs[i] < m.sortedSeqs[j] })
	}
	m.checkpoints[seq] = cp
	return true
}

// FindNearestCheckpoint locates the highest checkpoint sequence <= targetSeq.
func (m *StreamMemoizer) FindNearestCheckpoint(targetSeq uint64) (uint64, map[string]string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.sortedSeqs) == 0 {
		return 0, nil, false
	}

	// Binary search for greatest seq <= targetSeq
	idx := sort.Search(len(m.sortedSeqs), func(i int) bool {
		return m.sortedSeqs[i] > targetSeq
	})

	if idx == 0 {
		return 0, nil, false
	}

	nearestSeq := m.sortedSeqs[idx-1]
	state := m.checkpoints[nearestSeq]

	cp := make(map[string]string, len(state))
	for k, v := range state {
		cp[k] = v
	}
	return nearestSeq, cp, true
}

// Len returns the count of cached checkpoints.
func (m *StreamMemoizer) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.checkpoints)
}

// Clear flushes all cached checkpoints.
func (m *StreamMemoizer) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpoints = make(map[uint64]map[string]string)
	m.sortedSeqs = m.sortedSeqs[:0]
}

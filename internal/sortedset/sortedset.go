package sortedset

import (
	"sort"
	"sync"
)

type Entry[V any] struct {
	Member V
	Score  float64
}

type SortedSet[V any] struct {
	mu      sync.RWMutex
	entries []Entry[V]
}

func New[V any]() *SortedSet[V] {
	return &SortedSet[V]{}
}

func (ss *SortedSet[V]) Add(member V, score float64) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for i, e := range ss.entries {
		if score < e.Score {
			ss.entries = append(ss.entries, Entry[V]{})
			copy(ss.entries[i+1:], ss.entries[i:])
			ss.entries[i] = Entry[V]{Member: member, Score: score}
			return
		}
	}
	ss.entries = append(ss.entries, Entry[V]{Member: member, Score: score})
}

func (ss *SortedSet[V]) Remove(index int) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if index < 0 || index >= len(ss.entries) {
		return false
	}
	ss.entries = append(ss.entries[:index], ss.entries[index+1:]...)
	return true
}

func (ss *SortedSet[V]) Len() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.entries)
}

func (ss *SortedSet[V]) Get(index int) (Entry[V], bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if index < 0 || index >= len(ss.entries) {
		var zero Entry[V]
		return zero, false
	}
	return ss.entries[index], true
}

func (ss *SortedSet[V]) Range(start, end int) []Entry[V] {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if start < 0 {
		start = 0
	}
	if end > len(ss.entries) {
		end = len(ss.entries)
	}
	if start >= end {
		return nil
	}
	result := make([]Entry[V], end-start)
	copy(result, ss.entries[start:end])
	return result
}

func (ss *SortedSet[V]) Min() (Entry[V], bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if len(ss.entries) == 0 {
		var zero Entry[V]
		return zero, false
	}
	return ss.entries[0], true
}

func (ss *SortedSet[V]) Max() (Entry[V], bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if len(ss.entries) == 0 {
		var zero Entry[V]
		return zero, false
	}
	return ss.entries[len(ss.entries)-1], true
}

func (ss *SortedSet[V]) Contains(member V) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	for _, e := range ss.entries {
		if any(e.Member) == any(member) {
			return true
		}
	}
	return false
}

func (ss *SortedSet[V]) Index(member V) int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	for i, e := range ss.entries {
		if any(e.Member) == any(member) {
			return i
		}
	}
	return -1
}

func (ss *SortedSet[V]) Clear() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.entries = nil
}

func (ss *SortedSet[V]) Items() []Entry[V] {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	result := make([]Entry[V], len(ss.entries))
	copy(result, ss.entries)
	return result
}

func (ss *SortedSet[V]) SortByScore() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	sort.Slice(ss.entries, func(i, j int) bool {
		return ss.entries[i].Score < ss.entries[j].Score
	})
}

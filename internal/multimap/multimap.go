package multimap

import (
	"sort"
	"sync"
)

type MultiMap[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K][]V
}

func New[K comparable, V any]() *MultiMap[K, V] {
	return &MultiMap[K, V]{
		items: make(map[K][]V),
	}
}

func (m *MultiMap[K, V]) Add(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = append(m.items[key], value)
}

func (m *MultiMap[K, V]) Get(key K) []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vals := m.items[key]
	result := make([]V, len(vals))
	copy(result, vals)
	return result
}

func (m *MultiMap[K, V]) Remove(key K) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.items[key]
	delete(m.items, key)
	return ok
}

func (m *MultiMap[K, V]) RemoveValue(key K, value V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	vals := m.items[key]
	for i, v := range vals {
		if any(v) == any(value) {
			m.items[key] = append(vals[:i], vals[i+1:]...)
			if len(m.items[key]) == 0 {
				delete(m.items, key)
			}
			return true
		}
	}
	return false
}

func (m *MultiMap[K, V]) Contains(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.items[key]
	return ok
}

func (m *MultiMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for _, vals := range m.items {
		total += len(vals)
	}
	return total
}

func (m *MultiMap[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]K, 0, len(m.items))
	for k := range m.items {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return true
	})
	return keys
}

func (m *MultiMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[K][]V)
}

func (m *MultiMap[K, V]) ForEach(fn func(K, []V)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.items {
		vals := make([]V, len(v))
		copy(vals, v)
		fn(k, vals)
	}
}

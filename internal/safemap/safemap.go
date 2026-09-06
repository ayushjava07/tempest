package safemap

import (
	"sync"
)

type Map[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		items: make(map[K]V),
	}
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.items[key]
	return val, ok
}

func (m *Map[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
}

func (m *Map[K, V]) Delete(key K) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.items[key]
	delete(m.items, key)
	return ok
}

func (m *Map[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.items[key]
	return ok
}

func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *Map[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]K, 0, len(m.items))
	for k := range m.items {
		keys = append(keys, k)
	}
	return keys
}

func (m *Map[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vals := make([]V, 0, len(m.items))
	for _, v := range m.items {
		vals = append(vals, v)
	}
	return vals
}

func (m *Map[K, V]) Items() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[K]V, len(m.items))
	for k, v := range m.items {
		result[k] = v
	}
	return result
}

func (m *Map[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[K]V)
}

func (m *Map[K, V]) ForEach(fn func(K, V)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.items {
		fn(k, v)
	}
}

func (m *Map[K, V]) GetOrSet(key K, value V) V {
	m.mu.Lock()
	defer m.mu.Unlock()
	if val, ok := m.items[key]; ok {
		return val
	}
	m.items[key] = value
	return value
}

func (m *Map[K, V]) Update(key K, fn func(V) V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.items[key]
	if !ok {
		return false
	}
	m.items[key] = fn(val)
	return true
}

package cache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

type Cache[K comparable, V any] struct {
	mu       sync.RWMutex
	items    map[K]entry[V]
	ttl      time.Duration
	clock    func() time.Time
	onEvict  func(K, V)
	hits     int64
	misses   int64
	evictions int64
}

type Options struct {
	TTL     time.Duration
	Clock   func() time.Time
	OnEvict any
}

func New[K comparable, V any](opts Options) *Cache[K, V] {
	if opts.TTL <= 0 {
		opts.TTL = 5 * time.Minute
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	c := &Cache[K, V]{
		items: make(map[K]entry[V]),
		ttl:   opts.TTL,
		clock: opts.Clock,
	}
	if opts.OnEvict != nil {
		if fn, ok := opts.OnEvict.(func(K, V)); ok {
			c.onEvict = fn
		}
	}
	return c
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		var zero V
		return zero, false
	}
	if c.clock().After(e.expiresAt) {
		c.Delete(key)
		var zero V
		return zero, false
	}
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return e.value, true
}

func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry[V]{
		value:     value,
		expiresAt: c.clock().Add(c.ttl),
	}
}

func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry[V]{
		value:     value,
		expiresAt: c.clock().Add(ttl),
	}
}

func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		delete(c.items, key)
		c.evictions++
		if c.onEvict != nil {
			go c.onEvict(key, e.value)
		}
	}
}

func (c *Cache[K, V]) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.items {
		delete(c.items, k)
		c.evictions++
		if c.onEvict != nil {
			go c.onEvict(k, e.value)
		}
	}
}

func (c *Cache[K, V]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}

func (c *Cache[K, V]) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Stats{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		Size:      len(c.items),
	}
}

func (c *Cache[K, V]) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	removed := 0
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
			c.evictions++
			removed++
			if c.onEvict != nil {
				go c.onEvict(k, e.value)
			}
		}
	}
	return removed
}

func (c *Cache[K, V]) Keys() []K {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]K, 0, len(c.items))
	now := c.clock()
	for k, e := range c.items {
		if !now.After(e.expiresAt) {
			keys = append(keys, k)
		}
	}
	return keys
}

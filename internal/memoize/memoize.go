package memoize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCacheMiss = errors.New("memoize: entry not found in cache")
	ErrExpired   = errors.New("memoize: cache entry has expired")
)

// Entry encapsulates a cached step execution result.
type Entry struct {
	Key       string
	Namespace string
	Workflow  string
	StepID    string
	Output    map[string]any
	CreatedAt time.Time
	ExpiresAt time.Time
	HitCount  atomic.Uint64
}

// ComputeKey generates a deterministic SHA-256 hash key based on canonical input representation.
func ComputeKey(namespace, workflow, stepID string, input map[string]any) string {
	canonicalJSON := canonicalizeMap(input)
	raw := fmt.Sprintf("%s:%s:%s:%s", namespace, workflow, stepID, canonicalJSON)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func canonicalizeMap(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		val := m[k]
		if subMap, ok := val.(map[string]any); ok {
			pairs = append(pairs, fmt.Sprintf("%q:%s", k, canonicalizeMap(subMap)))
		} else {
			b, _ := json.Marshal(val)
			pairs = append(pairs, fmt.Sprintf("%q:%s", k, string(b)))
		}
	}
	return "{" + joinStrings(pairs, ",") + "}"
}

func joinStrings(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	out := elems[0]
	for _, s := range elems[1:] {
		out += sep + s
	}
	return out
}

// Cache provides thread-safe step result memoization with TTL and namespace invalidation.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]*Entry
	capacity int
	hits     atomic.Uint64
	misses   atomic.Uint64
}

func NewCache(capacity int) *Cache {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Cache{
		entries:  make(map[string]*Entry),
		capacity: capacity,
	}
}

// Get retrieves a cached output if present and not expired.
func (c *Cache) Get(ctx context.Context, key string) (map[string]any, error) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		c.misses.Add(1)
		return nil, ErrCacheMiss
	}

	if !entry.ExpiresAt.IsZero() && time.Now().UTC().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		c.misses.Add(1)
		return nil, ErrExpired
	}

	entry.HitCount.Add(1)
	c.hits.Add(1)

	// Return deep copy
	out := make(map[string]any, len(entry.Output))
	for k, v := range entry.Output {
		out[k] = v
	}
	return out, nil
}

// Put stores an execution result with the specified TTL.
func (c *Cache) Put(ctx context.Context, namespace, workflow, stepID string, input, output map[string]any, ttl time.Duration) string {
	key := ComputeKey(namespace, workflow, stepID, input)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction if capacity reached
	if len(c.entries) >= c.capacity {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}

	now := time.Now().UTC()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	cpOutput := make(map[string]any, len(output))
	for k, v := range output {
		cpOutput[k] = v
	}

	c.entries[key] = &Entry{
		Key:       key,
		Namespace: namespace,
		Workflow:  workflow,
		StepID:    stepID,
		Output:    cpOutput,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	return key
}

// InvalidateStep removes all cached entries for a specific step.
func (c *Cache) InvalidateStep(namespace, workflow, stepID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	deleted := 0
	for k, e := range c.entries {
		if e.Namespace == namespace && e.Workflow == workflow && e.StepID == stepID {
			delete(c.entries, k)
			deleted++
		}
	}
	return deleted
}

// InvalidateNamespace evicts all cached entries belonging to a namespace.
func (c *Cache) InvalidateNamespace(namespace string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	deleted := 0
	for k, e := range c.entries {
		if e.Namespace == namespace {
			delete(c.entries, k)
			deleted++
		}
	}
	return deleted
}

// Stats reports cache hits, misses, and current item count.
type Stats struct {
	Hits   uint64
	Misses uint64
	Size   int
}

func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Stats{
		Hits:   c.hits.Load(),
		Misses: c.misses.Load(),
		Size:   len(c.entries),
	}
}

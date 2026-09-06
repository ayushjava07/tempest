package cache

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	c := New[string, int](Options{TTL: time.Minute})
	c.Set("a", 42)
	v, ok := c.Get("a")
	if !ok || v != 42 {
		t.Errorf("expected 42, got %d, %v", v, ok)
	}
}

func TestCache_Expiry(t *testing.T) {
	now := time.Now()
	var t_ int64 = now.UnixNano()
	clock := func() time.Time { return time.Unix(0, atomic.LoadInt64(&t_)) }
	c := New[string, string](Options{TTL: time.Second, Clock: clock})
	c.Set("key", "val")
	v, ok := c.Get("key")
	if !ok || v != "val" {
		t.Fatal("expected hit before expiry")
	}
	atomic.StoreInt64(&t_, now.Add(2*time.Second).UnixNano())
	_, ok = c.Get("key")
	if ok {
		t.Error("expected miss after expiry")
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	c := New[string, int](Options{TTL: time.Minute})
	c.Set("a", 1)
	c.Set("b", 2)
	c.InvalidateAll()
	if c.Size() != 0 {
		t.Errorf("expected size 0, got %d", c.Size())
	}
}

func TestCache_OnEvict(t *testing.T) {
	var evicted atomic.Int32
	c := New[string, int](Options{
		TTL: time.Minute,
		OnEvict: func(k string, v int) {
			evicted.Add(1)
		},
	})
	c.Set("a", 1)
	c.Delete("a")
	time.Sleep(10 * time.Millisecond)
	if evicted.Load() != 1 {
		t.Errorf("expected 1 eviction, got %d", evicted.Load())
	}
}

func TestCache_Stats(t *testing.T) {
	c := New[string, int](Options{TTL: time.Minute})
	c.Set("a", 1)
	c.Get("a")
	c.Get("missing")
	s := c.Stats()
	if s.Hits != 1 || s.Misses != 1 {
		t.Errorf("expected 1 hit, 1 miss; got %v", s)
	}
}

func TestCache_SetWithTTL(t *testing.T) {
	now := time.Now()
	var t_ int64 = now.UnixNano()
	clock := func() time.Time { return time.Unix(0, atomic.LoadInt64(&t_)) }
	c := New[string, int](Options{TTL: time.Hour, Clock: clock})
	c.SetWithTTL("short", 1, 100*time.Millisecond)
	atomic.StoreInt64(&t_, now.Add(200*time.Millisecond).UnixNano())
	_, ok := c.Get("short")
	if ok {
		t.Error("expected miss after custom TTL")
	}
}

func TestCache_Cleanup(t *testing.T) {
	now := time.Now()
	var t_ int64 = now.UnixNano()
	clock := func() time.Time { return time.Unix(0, atomic.LoadInt64(&t_)) }
	c := New[string, int](Options{TTL: time.Second, Clock: clock})
	c.Set("a", 1)
	c.Set("b", 2)
	atomic.StoreInt64(&t_, now.Add(2*time.Second).UnixNano())
	removed := c.Cleanup()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
}

func TestCache_Keys(t *testing.T) {
	c := New[string, int](Options{TTL: time.Minute})
	c.Set("a", 1)
	c.Set("b", 2)
	keys := c.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := New[int, int](Options{TTL: time.Minute})
	var done atomic.Int32
	for i := 0; i < 100; i++ {
		go func(n int) {
			c.Set(n, n)
			c.Get(n)
			done.Add(1)
		}(i)
	}
	for done.Load() < 100 {
		time.Sleep(time.Millisecond)
	}
}

func TestCache_DefaultTTL(t *testing.T) {
	c := New[string, string](Options{})
	if c.ttl != 5*time.Minute {
		t.Errorf("expected default 5m, got %v", c.ttl)
	}
}

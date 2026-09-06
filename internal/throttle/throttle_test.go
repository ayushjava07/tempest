package throttle

import (
	"sync"
	"testing"
	"time"
)

func TestThrottle_Allow(t *testing.T) {
	th := New(time.Second, 3)
	if !th.Allow() {
		t.Error("first call should allow")
	}
	if !th.Allow() {
		t.Error("second call should allow (burst)")
	}
	if !th.Allow() {
		t.Error("third call should allow (burst)")
	}
	if th.Allow() {
		t.Error("fourth call should deny (burst exhausted)")
	}
}

func TestThrottle_Burst1(t *testing.T) {
	th := New(time.Second, 1)
	if !th.Allow() {
		t.Error("first should allow")
	}
	if th.Allow() {
		t.Error("second should deny")
	}
}

func TestThrottle_Reset(t *testing.T) {
	th := New(time.Second, 1)
	th.Allow()
	th.Reset()
	if !th.Allow() {
		t.Error("expected allow after reset")
	}
}

func TestThrottle_IntervalExpired(t *testing.T) {
	th := New(50*time.Millisecond, 1)
	th.Allow()
	time.Sleep(60 * time.Millisecond)
	if !th.Allow() {
		t.Error("expected allow after interval")
	}
}

func TestThrottle_Concurrent(t *testing.T) {
	th := New(50*time.Millisecond, 10)
	var wg sync.WaitGroup
	var allowed int
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if th.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Errorf("expected 10 allowed, got %d", allowed)
	}
}

func TestThrottle_Count(t *testing.T) {
	th := New(time.Second, 5)
	th.Allow()
	th.Allow()
	if th.Count() != 2 {
		t.Errorf("expected 2, got %d", th.Count())
	}
}

func TestThrottle_Meta(t *testing.T) {
	th := New(100*time.Millisecond, 3)
	if th.Interval() != 100*time.Millisecond {
		t.Error("wrong interval")
	}
	if th.Burst() != 3 {
		t.Error("wrong burst")
	}
}

func TestThrottle_Wait(t *testing.T) {
	th := New(10*time.Millisecond, 1)
	th.Allow()
	start := time.Now()
	th.Wait()
	if time.Since(start) < 5*time.Millisecond {
		t.Error("expected some wait")
	}
}

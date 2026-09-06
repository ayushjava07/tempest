package throttle

import (
	"sync"
	"time"
)

type Throttle struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
	burst    int
	count    int
}

func New(interval time.Duration, burst int) *Throttle {
	if burst <= 0 {
		burst = 1
	}
	return &Throttle{
		interval: interval,
		burst:    burst,
	}
}

func (t *Throttle) Allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) >= t.interval {
		t.last = now
		t.count = 1
		return true
	}
	if t.count < t.burst {
		t.count++
		return true
	}
	return false
}

func (t *Throttle) Wait() {
	for !t.Allow() {
		time.Sleep(t.interval / time.Duration(t.burst))
	}
}

func (t *Throttle) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.count = 0
	t.last = time.Time{}
}

func (t *Throttle) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

func (t *Throttle) Interval() time.Duration {
	return t.interval
}

func (t *Throttle) Burst() int {
	return t.burst
}

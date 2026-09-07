package quota

import (
	"sync"
	"time"
)

type Quota struct {
	mu       sync.Mutex
	limit    int64
	used     int64
	window   time.Duration
	start    time.Time
	refill   int64
	refillAt time.Time
}

func New(limit int64, window time.Duration, refill int64) *Quota {
	if limit <= 0 {
		limit = 1000
	}
	if window <= 0 {
		window = time.Hour
	}
	if refill <= 0 {
		refill = limit
	}
	return &Quota{
		limit:    limit,
		used:     0,
		window:   window,
		start:    time.Now(),
		refill:   refill,
		refillAt: time.Now().Add(window),
	}
}

func (q *Quota) Allow(n int64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.maybeRefill()
	if q.used+n <= q.limit {
		q.used += n
		return true
	}
	return false
}

func (q *Quota) maybeRefill() {
	now := time.Now()
	if now.After(q.refillAt) {
		q.used = 0
		q.refillAt = now.Add(q.window)
	}
}

func (q *Quota) Used() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.used
}

func (q *Quota) Remaining() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.maybeRefill()
	remaining := q.limit - q.used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (q *Quota) Limit() int64 {
	return q.limit
}

func (q *Quota) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.used = 0
	q.refillAt = time.Now().Add(q.window)
}

func (q *Quota) ResetAt(t time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.used = 0
	q.refillAt = t
}
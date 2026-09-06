package quota

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrQuotaExceeded     = errors.New("quota: resource quota exceeded")
	ErrRateLimitExceeded = errors.New("quota: rate limit exceeded")
	ErrInvalidLimit      = errors.New("quota: limit must be strictly positive")
	ErrReservationClosed = errors.New("quota: reservation already released")
)

// ResourceType classifies tracked system resources.
type ResourceType string

const (
	ResourceConcurrentRuns ResourceType = "concurrent_runs"
	ResourceQueueDepth     ResourceType = "queue_depth"
	ResourceDefinitions    ResourceType = "definitions"
	ResourceStepRate       ResourceType = "step_rate"
)

// TenantLimits defines quota constraints assigned to a tenant/namespace.
type TenantLimits struct {
	MaxConcurrentRuns int
	MaxQueueDepth     int
	MaxDefinitions    int
	MaxStepsPerSecond int
}

// DefaultLimits returns sensible baseline limits for standard tenants.
func DefaultLimits() TenantLimits {
	return TenantLimits{
		MaxConcurrentRuns: 50,
		MaxQueueDepth:     1000,
		MaxDefinitions:    200,
		MaxStepsPerSecond: 100,
	}
}

// Reservation represents an active allocation of quota held until released.
type Reservation interface {
	Resource() ResourceType
	Amount() int
	Release()
}

type reservationImpl struct {
	m        *Manager
	ns       string
	res      ResourceType
	amount   int
	released bool
	mu       sync.Mutex
}

func (r *reservationImpl) Resource() ResourceType { return r.res }
func (r *reservationImpl) Amount() int            { return r.amount }

func (r *reservationImpl) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.released {
		r.released = true
		r.m.release(r.ns, r.res, r.amount)
	}
}

// UsageReport summarizes current resource allocations against limits.
type UsageReport struct {
	Namespace      string
	ConcurrentRuns int
	MaxRuns        int
	QueueDepth     int
	MaxQueue       int
	Definitions    int
	MaxDefinitions int
}

// SlidingWindowLimiter implements smooth rate-limiting using sub-window buckets.
type SlidingWindowLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	subWindows  int
	buckets     []int
	bucketTimes []time.Time
	limit       int
}

func NewSlidingWindowLimiter(limit int, window time.Duration, subWindows int) *SlidingWindowLimiter {
	if subWindows <= 0 {
		subWindows = 10
	}
	return &SlidingWindowLimiter{
		window:      window,
		subWindows:  subWindows,
		buckets:     make([]int, subWindows),
		bucketTimes: make([]time.Time, subWindows),
		limit:       limit,
	}
}

func (s *SlidingWindowLimiter) Allow(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucketDuration := s.window / time.Duration(s.subWindows)
	currentBucketIdx := int((now.UnixNano() / bucketDuration.Nanoseconds()) % int64(s.subWindows))
	cutoff := now.Add(-s.window)

	// Clean expired buckets & sum active
	total := 0
	for i := 0; i < s.subWindows; i++ {
		if s.bucketTimes[i].Before(cutoff) {
			s.buckets[i] = 0
		} else {
			total += s.buckets[i]
		}
	}

	if total >= s.limit {
		return false
	}

	if s.bucketTimes[currentBucketIdx].Before(now.Add(-bucketDuration)) {
		s.buckets[currentBucketIdx] = 0
		s.bucketTimes[currentBucketIdx] = now
	}
	s.buckets[currentBucketIdx]++
	return true
}

// Manager controls quotas and rate limits across tenants.
type Manager struct {
	mu       sync.RWMutex
	limits   map[string]TenantLimits
	usage    map[string]map[ResourceType]int
	limiters map[string]*SlidingWindowLimiter
}

func NewManager() *Manager {
	return &Manager{
		limits:   make(map[string]TenantLimits),
		usage:    make(map[string]map[ResourceType]int),
		limiters: make(map[string]*SlidingWindowLimiter),
	}
}

func (m *Manager) SetLimits(namespace string, limits TenantLimits) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limits[namespace] = limits
	m.limiters[namespace] = NewSlidingWindowLimiter(limits.MaxStepsPerSecond, time.Second, 10)
}

func (m *Manager) getLimits(namespace string) TenantLimits {
	if l, ok := m.limits[namespace]; ok {
		return l
	}
	return DefaultLimits()
}

// Acquire reserves the requested amount of quota, returning a Reservation or ErrQuotaExceeded.
func (m *Manager) Acquire(ctx context.Context, namespace string, res ResourceType, amount int) (Reservation, error) {
	if amount <= 0 {
		return nil, ErrInvalidLimit
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	limits := m.getLimits(namespace)
	nsUsage, ok := m.usage[namespace]
	if !ok {
		nsUsage = make(map[ResourceType]int)
		m.usage[namespace] = nsUsage
	}

	current := nsUsage[res]
	var maxAllowed int
	switch res {
	case ResourceConcurrentRuns:
		maxAllowed = limits.MaxConcurrentRuns
	case ResourceQueueDepth:
		maxAllowed = limits.MaxQueueDepth
	case ResourceDefinitions:
		maxAllowed = limits.MaxDefinitions
	default:
		return nil, fmt.Errorf("unknown resource type: %s", res)
	}

	if current+amount > maxAllowed {
		return nil, fmt.Errorf("%w: %s exceeded (current %d + req %d > max %d)",
			ErrQuotaExceeded, res, current, amount, maxAllowed)
	}

	nsUsage[res] = current + amount
	return &reservationImpl{
		m:      m,
		ns:     namespace,
		res:    res,
		amount: amount,
	}, nil
}

func (m *Manager) release(namespace string, res ResourceType, amount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if nsUsage, ok := m.usage[namespace]; ok {
		nsUsage[res] -= amount
		if nsUsage[res] <= 0 {
			delete(nsUsage, res)
		}
	}
}

// AllowStep checks rate limiting for step executions within the namespace.
func (m *Manager) AllowStep(namespace string, now time.Time) bool {
	m.mu.Lock()
	limiter, ok := m.limiters[namespace]
	if !ok {
		limits := m.getLimits(namespace)
		limiter = NewSlidingWindowLimiter(limits.MaxStepsPerSecond, time.Second, 10)
		m.limiters[namespace] = limiter
	}
	m.mu.Unlock()

	return limiter.Allow(now)
}

// GetUsage generates a snapshot report of tenant resource usage.
func (m *Manager) GetUsage(namespace string) UsageReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limits := m.getLimits(namespace)
	nsUsage := m.usage[namespace]

	return UsageReport{
		Namespace:      namespace,
		ConcurrentRuns: nsUsage[ResourceConcurrentRuns],
		MaxRuns:        limits.MaxConcurrentRuns,
		QueueDepth:     nsUsage[ResourceQueueDepth],
		MaxQueue:       limits.MaxQueueDepth,
		Definitions:    nsUsage[ResourceDefinitions],
		MaxDefinitions: limits.MaxDefinitions,
	}
}

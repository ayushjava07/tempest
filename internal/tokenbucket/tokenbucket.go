package tokenbucket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

var (
	ErrRateLimitExceeded = errors.New("tokenbucket: rate limit exceeded")
	ErrLeaseExpired      = errors.New("tokenbucket: lease expired")
	ErrLeaseNotFound     = errors.New("tokenbucket: lease not found")
	ErrLimiterClosed     = errors.New("tokenbucket: limiter closed")
)

// Config defines capacity and refill rate.
type Config struct {
	Capacity   float64 // Maximum tokens in bucket
	RefillRate float64 // Tokens added per second
}

// Lease represents temporarily locked tokens with TTL protection.
type Lease struct {
	ID        string
	Tenant    string
	Tokens    float64
	ExpiresAt time.Time
	bucket    *Bucket
	released  bool
	mu        sync.Mutex
}

// Renew extends the lease TTL.
func (l *Lease) Renew(ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return ErrLeaseNotFound
	}
	if time.Now().After(l.ExpiresAt) {
		return ErrLeaseExpired
	}

	l.ExpiresAt = time.Now().Add(ttl)
	return nil
}

// Release yields the leased tokens back to the bucket.
func (l *Lease) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return nil
	}
	l.released = true
	l.bucket.returnTokens(l.Tokens, l.ID)
	return nil
}

// Bucket implements a token bucket rate limiter with leasing support.
type Bucket struct {
	mu         sync.Mutex
	cfg        Config
	tokens     float64
	lastRefill time.Time
	leases     map[string]*Lease
	wakeCh     chan struct{}
}

// NewBucket creates a token bucket initialized to full capacity.
func NewBucket(cfg Config) *Bucket {
	if cfg.Capacity <= 0 {
		cfg.Capacity = 1
	}
	if cfg.RefillRate <= 0 {
		cfg.RefillRate = 1
	}
	return &Bucket{
		cfg:        cfg,
		tokens:     cfg.Capacity,
		lastRefill: time.Now(),
		leases:     make(map[string]*Lease),
		wakeCh:     make(chan struct{}, 1),
	}
}

func (b *Bucket) refillLocked(now time.Time) {
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	b.tokens = math.Min(b.cfg.Capacity, b.tokens+elapsed*b.cfg.RefillRate)
	b.lastRefill = now
}

// Available returns the current number of available tokens.
func (b *Bucket) Available() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked(time.Now())
	return b.tokens
}

// TryAcquire attempts to immediately consume n tokens without blocking.
func (b *Bucket) TryAcquire(n float64) bool {
	if n <= 0 {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.refillLocked(time.Now())
	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

// Acquire blocks until n tokens become available or ctx is cancelled.
func (b *Bucket) Acquire(ctx context.Context, n float64) error {
	if n <= 0 {
		return nil
	}
	if n > b.cfg.Capacity {
		return fmt.Errorf("%w: requested tokens %f exceeds capacity %f", ErrRateLimitExceeded, n, b.cfg.Capacity)
	}

	for {
		b.mu.Lock()
		b.refillLocked(time.Now())

		if b.tokens >= n {
			b.tokens -= n
			b.mu.Unlock()
			return nil
		}

		// Calculate wait duration until sufficient tokens refill
		needed := n - b.tokens
		waitSec := needed / b.cfg.RefillRate
		waitDur := time.Duration(waitSec * float64(time.Second))
		b.mu.Unlock()

		timer := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-b.wakeCh:
			timer.Stop()
			// Tokens returned or added, retry
		case <-timer.C:
			// Timeout elapsed, retry loop
		}
	}
}

// Lease reserves n tokens for the given duration with automatic return on expiry.
func (b *Bucket) Lease(ctx context.Context, tenant string, n float64, ttl time.Duration) (*Lease, error) {
	if err := b.Acquire(ctx, n); err != nil {
		return nil, err
	}

	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	leaseID := hex.EncodeToString(randBytes)

	lease := &Lease{
		ID:        leaseID,
		Tenant:    tenant,
		Tokens:    n,
		ExpiresAt: time.Now().Add(ttl),
		bucket:    b,
	}

	b.mu.Lock()
	b.leases[leaseID] = lease
	b.mu.Unlock()

	return lease, nil
}

func (b *Bucket) returnTokens(n float64, leaseID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.leases, leaseID)
	b.refillLocked(time.Now())
	b.tokens = math.Min(b.cfg.Capacity, b.tokens+n)

	select {
	case b.wakeCh <- struct{}{}:
	default:
	}
}

// ReapExpiredLeases removes dead leases and restores their tokens.
func (b *Bucket) ReapExpiredLeases() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	var expiredCount int
	var reclaimedTokens float64

	for id, l := range b.leases {
		if now.After(l.ExpiresAt) {
			l.mu.Lock()
			l.released = true
			l.mu.Unlock()

			reclaimedTokens += l.Tokens
			delete(b.leases, id)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		b.refillLocked(now)
		b.tokens = math.Min(b.cfg.Capacity, b.tokens+reclaimedTokens)
		select {
		case b.wakeCh <- struct{}{}:
		default:
		}
	}

	return expiredCount
}

// MultiLimiter manages tenant-partitioned rate limits with background lease reaping.
type MultiLimiter struct {
	mu         sync.RWMutex
	defaultCfg Config
	buckets    map[string]*Bucket
	stopCh     chan struct{}
	wg         sync.WaitGroup
	running    bool
}

// NewMultiLimiter initializes a multi-tenant limiter.
func NewMultiLimiter(defaultCfg Config) *MultiLimiter {
	return &MultiLimiter{
		defaultCfg: defaultCfg,
		buckets:    make(map[string]*Bucket),
		stopCh:     make(chan struct{}),
	}
}

func (m *MultiLimiter) getOrCreateBucket(tenant string) *Bucket {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.buckets[tenant]
	if !ok {
		b = NewBucket(m.defaultCfg)
		m.buckets[tenant] = b
	}
	return b
}

// Acquire requests n tokens for the given tenant namespace.
func (m *MultiLimiter) Acquire(ctx context.Context, tenant string, n float64) error {
	b := m.getOrCreateBucket(tenant)
	return b.Acquire(ctx, n)
}

// Lease locks n tokens for the given tenant with TTL protection.
func (m *MultiLimiter) Lease(ctx context.Context, tenant string, n float64, ttl time.Duration) (*Lease, error) {
	b := m.getOrCreateBucket(tenant)
	return b.Lease(ctx, tenant, n, ttl)
}

// Stats returns the status of a tenant's bucket.
func (m *MultiLimiter) Stats(tenant string) (capacity, available, refillRate float64, activeLeases int) {
	m.mu.RLock()
	b, ok := m.buckets[tenant]
	m.mu.RUnlock()

	if !ok {
		return m.defaultCfg.Capacity, m.defaultCfg.Capacity, m.defaultCfg.RefillRate, 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked(time.Now())
	return b.cfg.Capacity, b.tokens, b.cfg.RefillRate, len(b.leases)
}

// StartReaper launches a background reaper for expired token leases.
func (m *MultiLimiter) StartReaper(interval time.Duration) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.mu.RLock()
				buckets := make([]*Bucket, 0, len(m.buckets))
				for _, b := range m.buckets {
					buckets = append(buckets, b)
				}
				m.mu.RUnlock()

				for _, b := range buckets {
					b.ReapExpiredLeases()
				}
			}
		}
	}()
}

// Stop terminates the reaper worker pool cleanly.
func (m *MultiLimiter) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	m.wg.Wait()
}

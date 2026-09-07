package throttler

import (
	"fmt"
	"sync"
	"time"
)

type logEntry struct {
	timestamps []time.Time
	config     ThrottleConfig
}

// SlidingLogThrottler enforces millisecond-accurate sliding-window rate limits.
type SlidingLogThrottler struct {
	mu     sync.RWMutex
	quotas map[string]*logEntry
}

// NewSlidingLogThrottler initializes a sliding-log rate limiter.
func NewSlidingLogThrottler() *SlidingLogThrottler {
	return &SlidingLogThrottler{
		quotas: make(map[string]*logEntry),
	}
}

// SetQuota registers or updates a rate limiting quota configuration.
func (t *SlidingLogThrottler) SetQuota(cfg ThrottleConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	entry, exists := t.quotas[cfg.QuotaKey]
	if !exists {
		t.quotas[cfg.QuotaKey] = &logEntry{
			timestamps: make([]time.Time, 0),
			config:     cfg,
		}
	} else {
		entry.config = cfg
	}
	return nil
}

// GetQuota returns the quota config for a key.
func (t *SlidingLogThrottler) GetQuota(key string) (ThrottleConfig, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	entry, exists := t.quotas[key]
	if !exists {
		return ThrottleConfig{}, false
	}
	return entry.config, true
}

// Allow evaluates a single operation against the rate limit.
func (t *SlidingLogThrottler) Allow(key string) (Decision, error) {
	return t.AllowN(key, 1)
}

// AllowN evaluates n operations against the sliding-log rate limit.
func (t *SlidingLogThrottler) AllowN(key string, n int) (Decision, error) {
	if n <= 0 {
		return Decision{Allowed: true}, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	entry, exists := t.quotas[key]
	if !exists {
		return Decision{}, fmt.Errorf("%w: %s", ErrQuotaNotFound, key)
	}

	now := time.Now()
	cutoff := now.Add(-entry.config.Window)

	// 1. Purge expired entries older than cutoff
	validIdx := 0
	for validIdx < len(entry.timestamps) && entry.timestamps[validIdx].Before(cutoff) {
		validIdx++
	}
	if validIdx > 0 {
		entry.timestamps = entry.timestamps[validIdx:]
	}

	currentCount := len(entry.timestamps)
	capacity := entry.config.RateLimit

	// 2. Evaluate admission
	if currentCount+n <= capacity {
		for i := 0; i < n; i++ {
			entry.timestamps = append(entry.timestamps, now)
		}

		remaining := capacity - (currentCount + n)
		var resetAfter time.Duration
		if len(entry.timestamps) > 0 {
			oldest := entry.timestamps[0]
			resetAfter = entry.config.Window - now.Sub(oldest)
			if resetAfter < 0 {
				resetAfter = 0
			}
		}

		return Decision{
			Allowed:    true,
			Remaining:  remaining,
			ResetAfter: resetAfter,
		}, nil
	}

	// 3. Rate limit exceeded: calculate wait time
	oldest := entry.timestamps[0]
	waitDuration := entry.config.Window - now.Sub(oldest)
	if waitDuration < time.Millisecond {
		waitDuration = time.Millisecond
	}

	return Decision{
		Allowed:      false,
		Remaining:    0,
		ResetAfter:   waitDuration,
		WaitDuration: waitDuration,
	}, nil
}

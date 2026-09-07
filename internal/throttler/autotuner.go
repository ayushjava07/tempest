package throttler

import (
	"math"
	"sync"
	"time"
)

// AutoTuner dynamically scales effective quota rates based on upstream HTTP 429 feedback.
type AutoTuner struct {
	mu             sync.Mutex
	throttler      *SlidingLogThrottler
	baseRates      map[string]int
	effectiveRates map[string]int
	successCounts  map[string]int
	coolOffUntil   map[string]time.Time
	decreaseFactor float64 // Multiplicative decrease (e.g. 0.5)
	minRate        int
	probeInterval  int // Number of consecutive successes needed to probe upward
}

// NewAutoTuner creates an adaptive auto-tuner wrapping a throttler.
func NewAutoTuner(t *SlidingLogThrottler, minRate int, decreaseFactor float64) *AutoTuner {
	if minRate <= 0 {
		minRate = 1
	}
	if decreaseFactor <= 0 || decreaseFactor >= 1.0 {
		decreaseFactor = 0.5
	}

	return &AutoTuner{
		throttler:      t,
		baseRates:      make(map[string]int),
		effectiveRates: make(map[string]int),
		successCounts:  make(map[string]int),
		coolOffUntil:   make(map[string]time.Time),
		decreaseFactor: decreaseFactor,
		minRate:        minRate,
		probeInterval:  20,
	}
}

// RegisterBaseQuota records baseline quota and configures the underlying throttler.
func (a *AutoTuner) RegisterBaseQuota(cfg ThrottleConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.baseRates[cfg.QuotaKey] = cfg.RateLimit
	a.effectiveRates[cfg.QuotaKey] = cfg.RateLimit
	return a.throttler.SetQuota(cfg)
}

// RecordFailure signals that upstream returned a rate-limiting status code (e.g. 429 or 503).
func (a *AutoTuner) RecordFailure(key string, statusCode int, retryAfter time.Duration) {
	if statusCode != 429 && statusCode != 503 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	base, exists := a.baseRates[key]
	if !exists {
		return
	}

	current := a.effectiveRates[key]
	// Multiplicative decrease
	newRate := int(math.Floor(float64(current) * a.decreaseFactor))
	if newRate < a.minRate {
		newRate = a.minRate
	}

	a.effectiveRates[key] = newRate
	a.successCounts[key] = 0

	if retryAfter > 0 {
		a.coolOffUntil[key] = time.Now().Add(retryAfter)
	}

	// Update underlying throttler quota
	cfg, ok := a.throttler.GetQuota(key)
	if ok {
		cfg.RateLimit = newRate
		_ = a.throttler.SetQuota(cfg)
	}
	_ = base
}

// RecordSuccess registers a successful upstream call, gradually restoring rate.
func (a *AutoTuner) RecordSuccess(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	base, exists := a.baseRates[key]
	if !exists {
		return
	}

	current := a.effectiveRates[key]
	if current >= base {
		return
	}

	a.successCounts[key]++
	if a.successCounts[key] >= a.probeInterval {
		// Additive increase
		newRate := current + 1
		if newRate > base {
			newRate = base
		}
		a.effectiveRates[key] = newRate
		a.successCounts[key] = 0

		cfg, ok := a.throttler.GetQuota(key)
		if ok {
			cfg.RateLimit = newRate
			_ = a.throttler.SetQuota(cfg)
		}
	}
}

// EffectiveRate returns the current dynamically tuned rate limit.
func (a *AutoTuner) EffectiveRate(key string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.effectiveRates[key]
}

// IsCoolingOff returns true if an upstream Retry-After backoff window is currently active.
func (a *AutoTuner) IsCoolingOff(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	deadline, exists := a.coolOffUntil[key]
	if !exists {
		return false
	}
	return time.Now().Before(deadline)
}

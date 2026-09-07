package adaptive

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrOverloaded       = errors.New("adaptive: system overloaded, concurrency limit reached")
	ErrAcquireTimeout   = errors.New("adaptive: acquire concurrency slot timed out")
	ErrTokenAlreadyUsed = errors.New("adaptive: token has already been released")
)

// Config configures the adaptive gradient concurrency limiter.
type Config struct {
	MinLimit     int
	MaxLimit     int
	InitialLimit int
	Smoothing    float64 // EMA smoothing factor (e.g. 0.2)
	Headroom     float64 // Additive headroom factor (e.g. 1.0)
	RttTolerance float64 // Tolerance multiplier on minRTT before decreasing (e.g. 1.25)
}

func DefaultConfig() Config {
	return Config{
		MinLimit:     5,
		MaxLimit:     100,
		InitialLimit: 20,
		Smoothing:    0.2,
		Headroom:     1.0,
		RttTolerance: 1.25,
	}
}

// Token represents a reserved concurrency slot held during execution.
type Token struct {
	limiter   *Limiter
	startTime time.Time
	released  atomic.Bool
}

func (t *Token) Release(err error) {
	if t.released.CompareAndSwap(false, true) {
		latency := time.Since(t.startTime)
		t.limiter.onSample(latency, err != nil)
		t.limiter.inFlight.Add(-1)
	}
}

// Limiter implements a gradient-based adaptive concurrency limiter.
type Limiter struct {
	mu           sync.Mutex
	cfg          Config
	currentLimit float64
	inFlight     atomic.Int64
	minRtt       time.Duration
	sampleRtt    time.Duration
	sampleCount  int
}

func NewLimiter(cfg Config) *Limiter {
	if cfg.MinLimit <= 0 {
		cfg.MinLimit = 5
	}
	if cfg.MaxLimit <= cfg.MinLimit {
		cfg.MaxLimit = cfg.MinLimit * 10
	}
	if cfg.InitialLimit < cfg.MinLimit || cfg.InitialLimit > cfg.MaxLimit {
		cfg.InitialLimit = cfg.MinLimit * 2
	}
	if cfg.Smoothing <= 0 || cfg.Smoothing > 1.0 {
		cfg.Smoothing = 0.2
	}
	if cfg.RttTolerance <= 0 {
		cfg.RttTolerance = 1.25
	}

	return &Limiter{
		cfg:          cfg,
		currentLimit: float64(cfg.InitialLimit),
	}
}

// Acquire reserves a concurrency slot, waiting if in-flight tasks exceed currentLimit.
func (l *Limiter) Acquire(ctx context.Context, timeout time.Duration) (*Token, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()

	for {
		l.mu.Lock()
		limit := int64(math.Round(l.currentLimit))
		inFlight := l.inFlight.Load()

		if inFlight < limit {
			l.inFlight.Add(1)
			l.mu.Unlock()
			return &Token{
				limiter:   l,
				startTime: time.Now(),
			}, nil
		}
		l.mu.Unlock()

		if timeout > 0 && time.Now().After(deadline) {
			return nil, ErrAcquireTimeout
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// TryAcquire attempts to reserve a slot immediately without waiting.
func (l *Limiter) TryAcquire() (*Token, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	limit := int64(math.Round(l.currentLimit))
	if l.inFlight.Load() >= limit {
		return nil, ErrOverloaded
	}

	l.inFlight.Add(1)
	return &Token{
		limiter:   l,
		startTime: time.Now(),
	}, nil
}

func (l *Limiter) onSample(latency time.Duration, isErr bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sampleCount++

	// Initialize or update minimum observed RTT
	if l.minRtt == 0 || latency < l.minRtt {
		l.minRtt = latency
	}

	// Update smoothed RTT using exponential moving average
	if l.sampleRtt == 0 {
		l.sampleRtt = latency
	} else {
		l.sampleRtt = time.Duration((1.0-l.cfg.Smoothing)*float64(l.sampleRtt) + l.cfg.Smoothing*float64(latency))
	}

	if isErr {
		// Drop limit sharply on error
		l.currentLimit *= 0.85
		if l.currentLimit < float64(l.cfg.MinLimit) {
			l.currentLimit = float64(l.cfg.MinLimit)
		}
		return
	}

	// Calculate gradient: minRTT / sampleRTT
	thresholdRtt := float64(l.minRtt) * l.cfg.RttTolerance
	gradient := thresholdRtt / float64(l.sampleRtt)
	if gradient > 1.0 {
		gradient = 1.0 // Cap gradient when latency is excellent
	}

	// Gradient formula: newLimit = currentLimit * gradient + headroom
	newLimit := l.currentLimit*gradient + l.cfg.Headroom

	// Clamp to configured bounds
	if newLimit < float64(l.cfg.MinLimit) {
		newLimit = float64(l.cfg.MinLimit)
	}
	if newLimit > float64(l.cfg.MaxLimit) {
		newLimit = float64(l.cfg.MaxLimit)
	}

	l.currentLimit = newLimit
}

// CurrentLimit returns the current dynamic concurrency ceiling.
func (l *Limiter) CurrentLimit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int(math.Round(l.currentLimit))
}

// InFlight returns the number of active concurrency reservations.
func (l *Limiter) InFlight() int {
	return int(l.inFlight.Load())
}

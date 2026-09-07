package throttler

import (
	"fmt"
	"sync"
	"time"
)

// BurstSmootherConfig configures dual-layer rate limiting: macro window and micro-burst smoothing.
type BurstSmootherConfig struct {
	QuotaKey       string        `json:"quota_key"`
	MacroRate      int           `json:"macro_rate"`       // e.g. 100 ops
	MacroWindow    time.Duration `json:"macro_window"`     // e.g. 1 second
	MicroBurstRate int           `json:"micro_burst_rate"` // e.g. 10 ops
	MicroWindow    time.Duration `json:"micro_window"`     // e.g. 50 milliseconds
}

// BurstSmoother prevents micro-burst spikes by enforcing both macro and micro rate bounds.
type BurstSmoother struct {
	mu       sync.Mutex
	cfg      BurstSmootherConfig
	macroLog []time.Time
	microLog []time.Time
}

// NewBurstSmoother creates a dual-layer burst smoothing rate limiter.
func NewBurstSmoother(cfg BurstSmootherConfig) (*BurstSmoother, error) {
	if cfg.MacroRate <= 0 || cfg.MicroBurstRate <= 0 {
		return nil, fmt.Errorf("%w: rates must be positive", ErrInvalidQuotaConfig)
	}
	if cfg.MacroWindow <= 0 || cfg.MicroWindow <= 0 {
		return nil, fmt.Errorf("%w: windows must be positive", ErrInvalidQuotaConfig)
	}
	if cfg.MicroWindow >= cfg.MacroWindow {
		return nil, fmt.Errorf("%w: micro window must be strictly smaller than macro window", ErrInvalidQuotaConfig)
	}

	return &BurstSmoother{
		cfg:      cfg,
		macroLog: make([]time.Time, 0),
		microLog: make([]time.Time, 0),
	}, nil
}

// Allow evaluates admission against both macro and micro burst windows.
func (b *BurstSmoother) Allow() (Decision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// 1. Prune macro window
	macroCutoff := now.Add(-b.cfg.MacroWindow)
	vIdx := 0
	for vIdx < len(b.macroLog) && b.macroLog[vIdx].Before(macroCutoff) {
		vIdx++
	}
	if vIdx > 0 {
		b.macroLog = b.macroLog[vIdx:]
	}

	// 2. Prune micro window
	microCutoff := now.Add(-b.cfg.MicroWindow)
	vIdx = 0
	for vIdx < len(b.microLog) && b.microLog[vIdx].Before(microCutoff) {
		vIdx++
	}
	if vIdx > 0 {
		b.microLog = b.microLog[vIdx:]
	}

	// 3. Check macro limit
	if len(b.macroLog) >= b.cfg.MacroRate {
		wait := b.cfg.MacroWindow - now.Sub(b.macroLog[0])
		return Decision{
			Allowed:      false,
			Remaining:    0,
			ResetAfter:   wait,
			WaitDuration: wait,
		}, nil
	}

	// 4. Check micro-burst limit
	if len(b.microLog) >= b.cfg.MicroBurstRate {
		wait := b.cfg.MicroWindow - now.Sub(b.microLog[0])
		return Decision{
			Allowed:      false,
			Remaining:    0,
			ResetAfter:   wait,
			WaitDuration: wait,
		}, nil
	}

	// Allowed by both!
	b.macroLog = append(b.macroLog, now)
	b.microLog = append(b.microLog, now)

	remMacro := b.cfg.MacroRate - len(b.macroLog)
	remMicro := b.cfg.MicroBurstRate - len(b.microLog)
	remaining := remMacro
	if remMicro < remaining {
		remaining = remMicro
	}

	return Decision{
		Allowed:   true,
		Remaining: remaining,
	}, nil
}

// EstimateWait predicts the expected queue delay given the number of items queued ahead.
func (b *BurstSmoother) EstimateWait(queuedAhead int) time.Duration {
	if queuedAhead <= 0 {
		return 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Calculate average service interval based on macro rate
	nsPerOp := float64(b.cfg.MacroWindow) / float64(b.cfg.MacroRate)
	estimatedWait := time.Duration(float64(queuedAhead) * nsPerOp)

	return estimatedWait
}

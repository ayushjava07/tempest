package chaos

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInjectedFault = errors.New("chaos: synthetic fault injected")
	ErrInjectedPanic = errors.New("chaos: synthetic panic injected")
)

// FaultType categorizes synthetic chaos anomalies.
type FaultType string

const (
	FaultLatency    FaultType = "LATENCY"
	FaultError      FaultType = "ERROR"
	FaultPanic      FaultType = "PANIC"
	FaultCorruption FaultType = "CORRUPTION"
)

// Rule configures when and how a chaos fault triggers.
type Rule struct {
	ID          string
	Type        FaultType
	Target      string        // Target identifier, or "*" for all
	Probability float64       // 0.0 to 1.0
	Latency     time.Duration // Duration for FaultLatency
	MaxHits     int           // Remaining occurrences before auto-disabling (0 = infinite)
	hits        *atomic.Int64
	Enabled     bool
}

// Interceptor evaluates and injects chaos faults during execution.
type Interceptor struct {
	mu      sync.RWMutex
	rules   map[string]*Rule
	enabled atomic.Bool
}

func NewInterceptor() *Interceptor {
	i := &Interceptor{
		rules: make(map[string]*Rule),
	}
	i.enabled.Store(true)
	return i
}

// Enable activates the chaos interceptor globally.
func (i *Interceptor) Enable() { i.enabled.Store(true) }

// Disable turns off all chaos injections globally.
func (i *Interceptor) Disable() { i.enabled.Store(false) }

// AddRule registers a new chaos injection rule.
func (i *Interceptor) AddRule(r *Rule) error {
	if r == nil || r.ID == "" {
		return errors.New("chaos: rule cannot be nil or have empty ID")
	}
	if r.Probability < 0.0 || r.Probability > 1.0 {
		return errors.New("chaos: probability must be between 0.0 and 1.0")
	}
	r.Enabled = true
	if r.hits == nil {
		r.hits = new(atomic.Int64)
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	i.rules[r.ID] = r
	return nil
}

// RemoveRule deletes a rule by ID.
func (i *Interceptor) RemoveRule(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.rules, id)
}

// Clear removes all registered rules.
func (i *Interceptor) Clear() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.rules = make(map[string]*Rule)
}

// Execute runs the provided function fn, intercepting with configured chaos rules.
func (i *Interceptor) Execute(ctx context.Context, target string, fn func(ctx context.Context) error) (err error) {
	if !i.enabled.Load() {
		return fn(ctx)
	}

	rule := i.matchRule(target)
	if rule == nil {
		return fn(ctx)
	}

	// Apply fault
	switch rule.Type {
	case FaultLatency:
		if rule.Latency > 0 {
			select {
			case <-time.After(rule.Latency):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return fn(ctx)

	case FaultError:
		return fmt.Errorf("%w: rule %s triggered on %s", ErrInjectedFault, rule.ID, target)

	case FaultPanic:
		panic(fmt.Sprintf("%s: rule %s triggered on %s", ErrInjectedPanic, rule.ID, target))

	default:
		return fn(ctx)
	}
}

func (i *Interceptor) matchRule(target string) *Rule {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, r := range i.rules {
		if !r.Enabled {
			continue
		}
		if r.Target != "*" && r.Target != target {
			continue
		}

		if r.MaxHits > 0 && r.hits.Load() >= int64(r.MaxHits) {
			continue
		}

		if r.Probability < 1.0 && rand.Float64() >= r.Probability {
			continue
		}

		r.hits.Add(1)
		return r
	}
	return nil
}

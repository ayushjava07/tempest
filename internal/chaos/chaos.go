package chaos

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type Fault interface {
	Inject(ctx context.Context) error
	Recover(ctx context.Context) error
	Name() string
}

type LatencyFault struct {
	duration time.Duration
	jitter   time.Duration
}

func NewLatencyFault(duration, jitter time.Duration) *LatencyFault {
	return &LatencyFault{duration: duration, jitter: jitter}
}

func (f *LatencyFault) Name() string { return "latency" }

func (f *LatencyFault) Inject(ctx context.Context) error {
	d := f.duration
	if f.jitter > 0 {
		d += time.Duration(rand.Int63n(int64(f.jitter)))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
	}
	return nil
}

func (f *LatencyFault) Recover(ctx context.Context) error { return nil }

type ErrorFault struct {
	err  error
	rate float64
}

func NewErrorFault(err error, rate float64) *ErrorFault {
	return &ErrorFault{err: err, rate: rate}
}

func (f *ErrorFault) Name() string { return "error" }

func (f *ErrorFault) Inject(ctx context.Context) error {
	if rand.Float64() < f.rate {
		return f.err
	}
	return nil
}

func (f *ErrorFault) Recover(ctx context.Context) error { return nil }

type ChaosEngine struct {
	mu      sync.RWMutex
	faults  map[string]Fault
	enabled bool
}

func NewEngine() *ChaosEngine {
	return &ChaosEngine{
		faults: make(map[string]Fault),
	}
}

func (e *ChaosEngine) Register(f Fault) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.faults[f.Name()] = f
}

func (e *ChaosEngine) Unregister(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.faults, name)
}

func (e *ChaosEngine) Enable() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = true
}

func (e *ChaosEngine) Disable() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = false
}

func (e *ChaosEngine) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

func (e *ChaosEngine) Execute(ctx context.Context, fn func() error) error {
	e.mu.RLock()
	enabled := e.enabled
	faults := make([]Fault, 0, len(e.faults))
	for _, f := range e.faults {
		faults = append(faults, f)
	}
	e.mu.RUnlock()

	if !enabled {
		return fn()
	}

	for _, f := range faults {
		if err := f.Inject(ctx); err != nil {
			return err
		}
	}
	return fn()
}

func (e *ChaosEngine) FaultCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.faults)
}
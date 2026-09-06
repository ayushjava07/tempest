package retry

import (
	"context"
	"math"
	"math/rand"
	"time"
)

type Policy struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxElapsed      time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts:     3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
	}
}

func (p Policy) Interval(attempt int) time.Duration {
	if attempt <= 0 {
		return p.InitialInterval
	}
	interval := float64(p.InitialInterval) * math.Pow(p.Multiplier, float64(attempt-1))
	if p.MaxInterval > 0 && time.Duration(interval) > p.MaxInterval {
		interval = float64(p.MaxInterval)
	}
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(interval * jitter)
}

type Result[T any] struct {
	Value    T
	Err      error
	Attempts int
}

func Do[T any](ctx context.Context, policy Policy, fn func(ctx context.Context) (T, error)) Result[T] {
	var lastErr error
	var lastVal T
	start := time.Now()
	for attempt := 0; attempt < policy.MaxAttempts || policy.MaxAttempts == 0; attempt++ {
		if ctx.Err() != nil {
			return Result[T]{Err: ctx.Err(), Attempts: attempt}
		}
		if attempt > 0 {
			interval := policy.Interval(attempt)
			select {
			case <-ctx.Done():
				return Result[T]{Err: ctx.Err(), Attempts: attempt}
			case <-time.After(interval):
			}
		}
		if policy.MaxElapsed > 0 && time.Since(start) > policy.MaxElapsed {
			return Result[T]{Value: lastVal, Err: lastErr, Attempts: attempt}
		}
		val, err := fn(ctx)
		if err == nil {
			return Result[T]{Value: val, Attempts: attempt + 1}
		}
		lastErr = err
		lastVal = val
	}
	return Result[T]{Value: lastVal, Err: lastErr, Attempts: policy.MaxAttempts}
}

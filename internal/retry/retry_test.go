package retry

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPolicy_Interval(t *testing.T) {
	p := Policy{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     5 * time.Second,
		Multiplier:      2.0,
	}
	// Attempt 0 returns initial interval (with jitter)
	interval := p.Interval(0)
	if interval < 50*time.Millisecond || interval > 200*time.Millisecond {
		t.Errorf("expected ~100ms, got %v", interval)
	}
	// Attempt 1: 100ms * 2^0 = 100ms (with jitter 0.8-1.2x = 80-120ms)
	interval = p.Interval(1)
	if interval < 80*time.Millisecond || interval > 120*time.Millisecond {
		t.Errorf("expected ~100ms, got %v", interval)
	}
	// Attempt 2: 100ms * 2^1 = 200ms (with jitter 0.8-1.2x = 160-240ms)
	interval = p.Interval(2)
	if interval < 160*time.Millisecond || interval > 240*time.Millisecond {
		t.Errorf("expected ~200ms, got %v", interval)
	}
}

func TestPolicy_MaxInterval(t *testing.T) {
	p := Policy{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     1 * time.Second,
		Multiplier:      10.0,
	}
	interval := p.Interval(5)
	if interval > 2*time.Second {
		t.Errorf("expected capped interval, got %v", interval)
	}
}

func TestDo_Success(t *testing.T) {
	p := Policy{MaxAttempts: 3}
	result := Do[string](context.Background(), p, func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
	if result.Value != "ok" {
		t.Errorf("expected ok, got %s", result.Value)
	}
	if result.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", result.Attempts)
	}
}

func TestDo_RetryThenSuccess(t *testing.T) {
	attempts := 0
	p := Policy{MaxAttempts: 3, InitialInterval: time.Millisecond}
	result := Do[string](context.Background(), p, func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", fmt.Errorf("attempt %d", attempts)
		}
		return "ok", nil
	})
	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
	if result.Value != "ok" {
		t.Errorf("expected ok, got %s", result.Value)
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	p := Policy{MaxAttempts: 3, InitialInterval: time.Millisecond}
	result := Do[string](context.Background(), p, func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("always fail")
	})
	if result.Err == nil {
		t.Error("expected error")
	}
	if result.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", result.Attempts)
	}
}

func TestDo_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := Policy{MaxAttempts: 100, InitialInterval: time.Millisecond}
	result := Do[string](ctx, p, func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("should not reach here")
	})
	if result.Err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", result.Err)
	}
}

func TestDo_MaxElapsed(t *testing.T) {
	p := Policy{
		MaxAttempts:     100,
		InitialInterval: 10 * time.Millisecond,
		MaxElapsed:      50 * time.Millisecond,
	}
	start := time.Now()
	result := Do[string](context.Background(), p, func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("always fail")
	})
	if time.Since(start) > 200*time.Millisecond {
		t.Error("expected to stop early due to MaxElapsed")
	}
	if result.Err == nil {
		t.Error("expected error")
	}
}

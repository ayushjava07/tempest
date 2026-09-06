package chaos

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestChaos_LatencyFault(t *testing.T) {
	inter := NewInterceptor()
	_ = inter.AddRule(&Rule{
		ID:          "latency-rule",
		Type:        FaultLatency,
		Target:      "database:query",
		Probability: 1.0,
		Latency:     50 * time.Millisecond,
	})

	start := time.Now()
	err := inter.Execute(context.Background(), "database:query", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Errorf("expected at least 50ms latency, got %v", elapsed)
	}
}

func TestChaos_ErrorFault(t *testing.T) {
	inter := NewInterceptor()
	_ = inter.AddRule(&Rule{
		ID:          "err-rule",
		Type:        FaultError,
		Target:      "step:flaky",
		Probability: 1.0,
	})

	// Target matches
	err := inter.Execute(context.Background(), "step:flaky", func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrInjectedFault) {
		t.Errorf("expected ErrInjectedFault, got %v", err)
	}

	// Target does not match
	err = inter.Execute(context.Background(), "step:other", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil for non-matching target, got %v", err)
	}
}

func TestChaos_MaxHits(t *testing.T) {
	inter := NewInterceptor()
	_ = inter.AddRule(&Rule{
		ID:          "countdown-rule",
		Type:        FaultError,
		Target:      "test",
		Probability: 1.0,
		MaxHits:     2,
	})

	// Hit 1: fails
	err1 := inter.Execute(context.Background(), "test", func(ctx context.Context) error { return nil })
	if !errors.Is(err1, ErrInjectedFault) {
		t.Errorf("hit 1 expected ErrInjectedFault, got %v", err1)
	}

	// Hit 2: fails
	err2 := inter.Execute(context.Background(), "test", func(ctx context.Context) error { return nil })
	if !errors.Is(err2, ErrInjectedFault) {
		t.Errorf("hit 2 expected ErrInjectedFault, got %v", err2)
	}

	// Hit 3: should succeed because maxHits=2 reached
	err3 := inter.Execute(context.Background(), "test", func(ctx context.Context) error { return nil })
	if err3 != nil {
		t.Errorf("hit 3 should succeed after maxHits reached, got %v", err3)
	}
}

func TestChaos_PanicFault(t *testing.T) {
	inter := NewInterceptor()
	_ = inter.AddRule(&Rule{
		ID:          "panic-rule",
		Type:        FaultPanic,
		Target:      "panic:target",
		Probability: 1.0,
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic from FaultPanic")
		}
	}()

	_ = inter.Execute(context.Background(), "panic:target", func(ctx context.Context) error {
		return nil
	})
}

func TestChaos_GlobalDisable(t *testing.T) {
	inter := NewInterceptor()
	_ = inter.AddRule(&Rule{
		ID:          "rule-disabled",
		Type:        FaultError,
		Target:      "*",
		Probability: 1.0,
	})

	inter.Disable()

	err := inter.Execute(context.Background(), "any-target", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no fault when globally disabled, got %v", err)
	}
}

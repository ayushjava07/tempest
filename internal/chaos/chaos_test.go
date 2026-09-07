package chaos

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLatencyFault(t *testing.T) {
	f := NewLatencyFault(10*time.Millisecond, 5*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := f.Inject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected at least 10ms, got %v", elapsed)
	}
}

func TestErrorFault(t *testing.T) {
	f := NewErrorFault(errors.New("injected"), 1.0)
	err := f.Inject(context.Background())
	if err == nil {
		t.Error("expected error")
	}
	if err.Error() != "injected" {
		t.Errorf("expected 'injected', got %s", err.Error())
	}
}

func TestErrorFault_Rate(t *testing.T) {
	f := NewErrorFault(errors.New("fail"), 0.0)
	for i := 0; i < 100; i++ {
		if err := f.Inject(context.Background()); err != nil {
			t.Error("expected no error at 0% rate")
		}
	}
}

func TestEngine_RegisterUnregister(t *testing.T) {
	e := NewEngine()
	f := NewLatencyFault(time.Millisecond, 0)
	e.Register(f)
	if e.FaultCount() != 1 {
		t.Error("expected 1 fault")
	}
	e.Unregister("latency")
	if e.FaultCount() != 0 {
		t.Error("expected 0 faults")
	}
}

func TestEngine_EnableDisable(t *testing.T) {
	e := NewEngine()
	if e.IsEnabled() {
		t.Error("expected disabled by default")
	}
	e.Enable()
	if !e.IsEnabled() {
		t.Error("expected enabled")
	}
	e.Disable()
	if e.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestEngine_ExecuteWithFaults(t *testing.T) {
	e := NewEngine()
	e.Register(NewErrorFault(errors.New("chaos"), 1.0))
	e.Enable()
	err := e.Execute(context.Background(), func() error { return nil })
	if err == nil {
		t.Error("expected fault error")
	}
}

func TestEngine_ExecuteDisabled(t *testing.T) {
	e := NewEngine()
	e.Register(NewErrorFault(errors.New("chaos"), 1.0))
	err := e.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Error("expected no error when disabled")
	}
}

func TestEngine_ExecuteWithLatency(t *testing.T) {
	e := NewEngine()
	e.Register(NewLatencyFault(5*time.Millisecond, 0))
	e.Enable()
	start := time.Now()
	err := e.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Error("expected latency injection")
	}
}
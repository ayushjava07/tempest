package hook

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegistry_RegisterExecute(t *testing.T) {
	r := NewRegistry[string]()
	var called atomic.Bool
	r.Register("test", 1, func(ctx context.Context, event string) error {
		called.Store(true)
		return nil
	})
	if err := r.Execute(context.Background(), "event"); err != nil {
		t.Fatal(err)
	}
	if !called.Load() {
		t.Error("expected hook called")
	}
}

func TestRegistry_Priority(t *testing.T) {
	r := NewRegistry[string]()
	var order []string
	r.Register("low", 10, func(ctx context.Context, event string) error {
		order = append(order, "low")
		return nil
	})
	r.Register("high", 1, func(ctx context.Context, event string) error {
		order = append(order, "high")
		return nil
	})
	r.Execute(context.Background(), "event")
	if len(order) != 2 || order[0] != "high" {
		t.Error("expected high priority first")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry[string]()
	var called atomic.Bool
	r.Register("test", 1, func(ctx context.Context, event string) error {
		called.Store(true)
		return nil
	})
	r.Unregister("test")
	r.Execute(context.Background(), "event")
	if called.Load() {
		t.Error("expected hook not called after unregister")
	}
}

func TestRegistry_HookError(t *testing.T) {
	r := NewRegistry[string]()
	r.Register("fail", 1, func(ctx context.Context, event string) error {
		return fmt.Errorf("hook error")
	})
	err := r.Execute(context.Background(), "event")
	if err == nil {
		t.Error("expected error")
	}
}

func TestRegistry_MultipleHooksSameName(t *testing.T) {
	r := NewRegistry[string]()
	var count atomic.Int32
	r.Register("multi", 1, func(ctx context.Context, event string) error {
		count.Add(1)
		return nil
	})
	r.Register("multi", 1, func(ctx context.Context, event string) error {
		count.Add(1)
		return nil
	})
	r.Execute(context.Background(), "event")
	if count.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", count.Load())
	}
}

func TestRegistry_ExecuteAsync(t *testing.T) {
	r := NewRegistry[string]()
	var called atomic.Bool
	r.Register("async", 1, func(ctx context.Context, event string) error {
		called.Store(true)
		return nil
	})
	r.ExecuteAsync(context.Background(), "event")
	time.Sleep(10 * time.Millisecond)
	if !called.Load() {
		t.Error("expected async hook called")
	}
}

func TestRegistry_HookCount(t *testing.T) {
	r := NewRegistry[string]()
	r.Register("a", 1, func(ctx context.Context, event string) error { return nil })
	r.Register("b", 1, func(ctx context.Context, event string) error { return nil })
	r.Register("b", 1, func(ctx context.Context, event string) error { return nil })
	if r.HookCount() != 3 {
		t.Errorf("expected 3, got %d", r.HookCount())
	}
}

func TestRegistry_EmptyExecute(t *testing.T) {
	r := NewRegistry[string]()
	if err := r.Execute(context.Background(), "event"); err != nil {
		t.Error("expected no error for empty registry")
	}
}
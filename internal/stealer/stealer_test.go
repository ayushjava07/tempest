package stealer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkStealer_PushPopLocal(t *testing.T) {
	ws := NewWorkStealer[int](10)
	ws.PushLocal(1)
	ws.PushLocal(2)
	v, ok := ws.PopLocal()
	if !ok || v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
	v, ok = ws.PopLocal()
	if !ok || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestWorkStealer_Steal(t *testing.T) {
	ws := NewWorkStealer[int](10)
	ws.PushLocal(1)
	ws.PushLocal(2)
	v, ok := ws.Steal()
	if !ok || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
	v, ok = ws.PopLocal()
	if !ok || v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
}

func TestWorkStealer_TryStealRemote(t *testing.T) {
	ws := NewWorkStealer[int](10)
	ws.PushRemote(42)
	v, ok := ws.TrySteal()
	if !ok || v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestWorkStealer_Empty(t *testing.T) {
	ws := NewWorkStealer[int](10)
	_, ok := ws.PopLocal()
	if ok {
		t.Error("expected false")
	}
	_, ok = ws.Steal()
	if ok {
		t.Error("expected false")
	}
}

func TestWorkStealer_Len(t *testing.T) {
	ws := NewWorkStealer[int](10)
	ws.PushLocal(1)
	ws.PushLocal(2)
	if ws.Len() != 2 {
		t.Errorf("expected 2, got %d", ws.Len())
	}
}

func TestWorkStealer_Close(t *testing.T) {
	ws := NewWorkStealer[int](10)
	ws.Close()
	ws.PushLocal(1)
	if ws.Len() != 0 {
		t.Error("expected closed to prevent push")
	}
}

func TestWorkStealer_Run(t *testing.T) {
	ws := NewWorkStealer[int](10)
	for i := 0; i < 10; i++ {
		ws.PushLocal(i)
	}
	var processed atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := ws.Run(ctx, func(v int) error {
		processed.Add(1)
		return nil
	})
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if processed.Load() < 10 {
		t.Errorf("expected at least 10 processed, got %d", processed.Load())
	}
}
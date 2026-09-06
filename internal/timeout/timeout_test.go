package timeout

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRun_Success(t *testing.T) {
	err := Run(context.Background(), time.Second, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_Timeout(t *testing.T) {
	err := Run(context.Background(), 10*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestRunWithResult(t *testing.T) {
	result, err := RunWithResult(context.Background(), time.Second, func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestOrTimeout(t *testing.T) {
	ctx, cancel := OrTimeout(context.Background(), time.Second)
	defer cancel()
	if ctx.Err() != nil {
		t.Error("expected no error yet")
	}
}

func TestTimer_Stop(t *testing.T) {
	timer := NewTimer(time.Hour)
	if !timer.Stop() {
		t.Error("expected stopped")
	}
}

func TestTimer_Reset(t *testing.T) {
	timer := NewTimer(time.Hour)
	timer.Reset(50 * time.Millisecond)
	select {
	case <-timer.done:
	case <-time.After(time.Second):
		t.Error("expected timer to fire")
	}
}

func TestAfterFunc(t *testing.T) {
	var called atomic.Bool
	AfterFunc(50*time.Millisecond, func() {
		called.Store(true)
	})
	time.Sleep(100 * time.Millisecond)
	if !called.Load() {
		t.Error("expected AfterFunc to be called")
	}
}

func TestRun_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, time.Second, func(ctx context.Context) error {
		return ctx.Err()
	})
	if err != context.Canceled {
		t.Errorf("expected Canceled, got %v", err)
	}
}

func TestRunWithResult_Timeout(t *testing.T) {
	_, err := RunWithResult(context.Background(), 10*time.Millisecond, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

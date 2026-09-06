package shutdown

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func TestManager_Register(t *testing.T) {
	m := New()
	called := false
	m.Register("test", 1, func(ctx context.Context) error {
		called = true
		return nil
	})
	if len(m.sortedHooks()) != 1 {
		t.Error("expected 1 hook")
	}
	_ = called
}

func TestManager_SortedHooks(t *testing.T) {
	m := New()
	m.Register("c", 3, func(ctx context.Context) error { return nil })
	m.Register("a", 1, func(ctx context.Context) error { return nil })
	m.Register("b", 2, func(ctx context.Context) error { return nil })
	hooks := m.sortedHooks()
	if hooks[0].Name != "a" || hooks[1].Name != "b" || hooks[2].Name != "c" {
		t.Error("expected sorted order")
	}
}

func TestManager_ContextCancel(t *testing.T) {
	m := New(syscall.SIGUSR1)
	ctx, cancel := context.WithCancel(context.Background())
	m.Listen(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestManager_StopCh(t *testing.T) {
	m := New(syscall.SIGUSR1)
	ctx := context.Background()
	_ = m.Listen(ctx)
	select {
	case <-m.StopCh():
	case <-time.After(100 * time.Millisecond):
	}
}

func TestManager_DuplicateListen(t *testing.T) {
	m := New(syscall.SIGUSR1)
	ctx := context.Background()
	_ = m.Listen(ctx)
	_ = m.Listen(ctx)
}

func TestManager_HookExecution(t *testing.T) {
	m := New(syscall.SIGUSR1)
	var order []string
	m.Register("first", 1, func(ctx context.Context) error {
		order = append(order, "first")
		return nil
	})
	m.Register("second", 2, func(ctx context.Context) error {
		order = append(order, "second")
		return nil
	})
	hooks := m.sortedHooks()
	for _, h := range hooks {
		_ = h.Func(context.Background())
	}
	if len(order) != 2 || order[0] != "first" {
		t.Error("expected hooks in priority order")
	}
}

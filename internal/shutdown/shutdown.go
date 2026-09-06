package shutdown

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Hook struct {
	Name     string
	Priority int
	Func     func(ctx context.Context) error
}

type Manager struct {
	mu        sync.Mutex
	hooks     []Hook
	signals   []os.Signal
	listening bool
	stopCh    chan struct{}
}

func New(signals ...os.Signal) *Manager {
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}
	return &Manager{
		signals: signals,
		stopCh:  make(chan struct{}),
	}
}

func (m *Manager) Register(name string, priority int, fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, Hook{Name: name, Priority: priority, Func: fn})
}

func (m *Manager) Listen(ctx context.Context) context.Context {
	m.mu.Lock()
	if m.listening {
		m.mu.Unlock()
		return ctx
	}
	m.listening = true
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, m.signals...)

	go func() {
		select {
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "received signal %v, shutting down\n", sig)
			m.runHooks(ctx)
			cancel()
			close(m.stopCh)
		case <-ctx.Done():
			close(m.stopCh)
		}
	}()

	return ctx
}

func (m *Manager) runHooks(ctx context.Context) {
	sorted := m.sortedHooks()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, hook := range sorted {
		fmt.Fprintf(os.Stderr, "shutting down: %s\n", hook.Name)
		if err := hook.Func(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error in %s: %v\n", hook.Name, err)
		}
	}
}

func (m *Manager) sortedHooks() []Hook {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Hook, len(m.hooks))
	copy(result, m.hooks)
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Priority < result[i].Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func (m *Manager) StopCh() <-chan struct{} {
	return m.stopCh
}

func (m *Manager) TriggerShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, m.signals...)
}

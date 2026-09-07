package hook

import (
	"context"
	"fmt"
	"sync"
)

type Hook[T any] func(ctx context.Context, event T) error

type Registry[T any] struct {
	mu      sync.RWMutex
	hooks   map[string][]Hook[T]
	priorities map[string]int
}

func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{
		hooks:      make(map[string][]Hook[T]),
		priorities: make(map[string]int),
	}
}

func (r *Registry[T]) Register(name string, priority int, hook Hook[T]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[name] = append(r.hooks[name], hook)
	r.priorities[name] = priority
}

func (r *Registry[T]) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.hooks, name)
	delete(r.priorities, name)
}

func (r *Registry[T]) Execute(ctx context.Context, event T) error {
	r.mu.RLock()
	names := make([]string, 0, len(r.hooks))
	for n := range r.hooks {
		names = append(names, n)
	}
	hooks := make(map[string][]Hook[T])
	for n, h := range r.hooks {
		hooks[n] = h
	}
	priorities := make(map[string]int)
	for n, p := range r.priorities {
		priorities[n] = p
	}
	r.mu.RUnlock()
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if priorities[names[j]] < priorities[names[i]] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	for _, name := range names {
		for _, hook := range hooks[name] {
			if err := hook(ctx, event); err != nil {
				return fmt.Errorf("hook %s: %w", name, err)
			}
		}
	}
	return nil
}

func (r *Registry[T]) ExecuteAsync(ctx context.Context, event T) {
	go r.Execute(ctx, event)
}

func (r *Registry[T]) HookCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, h := range r.hooks {
		total += len(h)
	}
	return total
}
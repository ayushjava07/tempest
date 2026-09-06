package pool

import (
	"sync"
)

type Pool[T any] struct {
	mu      sync.Mutex
	items   []T
	factory func() T
	reset   func(T)
	maxSize int
}

func New[T any](maxSize int, factory func() T, reset func(T)) *Pool[T] {
	if maxSize <= 0 {
		maxSize = 64
	}
	return &Pool[T]{
		items:   make([]T, 0, maxSize),
		factory: factory,
		reset:   reset,
		maxSize: maxSize,
	}
}

func (p *Pool[T]) Get() T {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.items) > 0 {
		item := p.items[len(p.items)-1]
		p.items = p.items[:len(p.items)-1]
		return item
	}
	return p.factory()
}

func (p *Pool[T]) Put(item T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reset != nil {
		p.reset(item)
	}
	if len(p.items) < p.maxSize {
		p.items = append(p.items, item)
	}
}

func (p *Pool[T]) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items)
}

func (p *Pool[T]) Cap() int {
	return p.maxSize
}

func (p *Pool[T]) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = p.items[:0]
}

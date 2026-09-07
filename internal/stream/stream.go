package stream

import (
	"context"
	"fmt"
	"sync"
)

type Processor[T any] func(ctx context.Context, item T) error

type Pipeline[T any] struct {
	mu         sync.Mutex
	processors []Processor[T]
}

func NewPipeline[T any]() *Pipeline[T] {
	return &Pipeline[T]{}
}

func (p *Pipeline[T]) Add(processor Processor[T]) *Pipeline[T] {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.processors = append(p.processors, processor)
	return p
}

func (p *Pipeline[T]) Process(ctx context.Context, item T) error {
	p.mu.Lock()
	procs := make([]Processor[T], len(p.processors))
	copy(procs, p.processors)
	p.mu.Unlock()
	for _, proc := range procs {
		if err := proc(ctx, item); err != nil {
			return fmt.Errorf("pipeline error: %w", err)
		}
	}
	return nil
}

func (p *Pipeline[T]) ProcessBatch(ctx context.Context, items []T) error {
	for _, item := range items {
		if err := p.Process(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

type Buffer[T any] struct {
	mu       sync.Mutex
	items    []T
	capacity int
	cond     *sync.Cond
	closed   bool
}

func NewBuffer[T any](capacity int) *Buffer[T] {
	b := &Buffer[T]{
		items:    make([]T, 0, capacity),
		capacity: capacity,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *Buffer[T]) Push(item T) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("buffer closed")
	}
	for len(b.items) >= b.capacity {
		b.cond.Wait()
		if b.closed {
			return fmt.Errorf("buffer closed")
		}
	}
	b.items = append(b.items, item)
	b.cond.Signal()
	return nil
}

func (b *Buffer[T]) Pop() (T, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.items) == 0 {
		if b.closed {
			var zero T
			return zero, fmt.Errorf("buffer closed")
		}
		b.cond.Wait()
	}
	item := b.items[0]
	b.items = b.items[1:]
	b.cond.Signal()
	return item, nil
}

func (b *Buffer[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.cond.Broadcast()
}

func (b *Buffer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

func (b *Buffer[T]) Cap() int {
	return b.capacity
}

type FanOut[T any] struct {
	mu        sync.RWMutex
	channels  []chan T
	closed    bool
}

func NewFanOut[T any](count int, bufferSize int) *FanOut[T] {
	f := &FanOut[T]{
		channels: make([]chan T, count),
	}
	for i := 0; i < count; i++ {
		f.channels[i] = make(chan T, bufferSize)
	}
	return f
}

func (f *FanOut[T]) Send(item T) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return fmt.Errorf("fanout closed")
	}
	for _, ch := range f.channels {
		select {
		case ch <- item:
		default:
			return fmt.Errorf("channel full")
		}
	}
	return nil
}

func (f *FanOut[T]) Channel(index int) (<-chan T, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if index < 0 || index >= len(f.channels) {
		return nil, fmt.Errorf("invalid index")
	}
	return f.channels[index], nil
}

func (f *FanOut[T]) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.closed = true
	for _, ch := range f.channels {
		close(ch)
	}
}
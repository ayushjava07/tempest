package stealer

import (
	"context"
	"sync"
	"time"
)

type WorkStealer[T any] struct {
	mu       sync.Mutex
	local    []T
	remote   chan T
	stealCh  chan struct{}
	closed   bool
}

func NewWorkStealer[T any](bufferSize int) *WorkStealer[T] {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &WorkStealer[T]{
		local:   make([]T, 0, bufferSize),
		remote:  make(chan T, bufferSize),
		stealCh: make(chan struct{}, 1),
	}
}

func (ws *WorkStealer[T]) PushLocal(item T) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.closed {
		return
	}
	ws.local = append(ws.local, item)
}

func (ws *WorkStealer[T]) PopLocal() (T, bool) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if len(ws.local) == 0 {
		var zero T
		return zero, false
	}
	item := ws.local[len(ws.local)-1]
	ws.local = ws.local[:len(ws.local)-1]
	return item, true
}

func (ws *WorkStealer[T]) PushRemote(item T) {
	select {
	case ws.remote <- item:
	default:
	}
}

func (ws *WorkStealer[T]) Steal() (T, bool) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if len(ws.local) == 0 {
		var zero T
		return zero, false
	}
	item := ws.local[0]
	ws.local = ws.local[1:]
	return item, true
}

func (ws *WorkStealer[T]) TrySteal() (T, bool) {
	select {
	case item := <-ws.remote:
		return item, true
	default:
		var zero T
		return zero, false
	}
}

func (ws *WorkStealer[T]) Len() int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return len(ws.local)
}

func (ws *WorkStealer[T]) RemoteLen() int {
	return len(ws.remote)
}

func (ws *WorkStealer[T]) Close() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.closed = true
	close(ws.remote)
}

func (ws *WorkStealer[T]) Run(ctx context.Context, workerFn func(T) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		item, ok := ws.PopLocal()
		if !ok {
			item, ok = ws.TrySteal()
			if !ok {
				time.Sleep(time.Millisecond)
				continue
			}
		}
		if err := workerFn(item); err != nil {
			return err
		}
	}
}
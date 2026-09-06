package broadcast

import (
	"sync"
)

type Broadcast[T any] struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan T
	nextID      uint64
}

func New[T any]() *Broadcast[T] {
	return &Broadcast[T]{
		subscribers: make(map[uint64]chan T),
	}
}

func (b *Broadcast[T]) Subscribe(bufferSize int) (uint64, <-chan T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan T, bufferSize)
	b.subscribers[id] = ch
	return id, ch
}

func (b *Broadcast[T]) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
	}
}

func (b *Broadcast[T]) Publish(value T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- value:
		default:
		}
	}
}

func (b *Broadcast[T]) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

func (b *Broadcast[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
}

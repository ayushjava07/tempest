package ringbuffer

import (
	"sync"
)

type RingBuffer[T any] struct {
	mu     sync.Mutex
	buf    []T
	head   int
	tail   int
	count  int
	maxCap int
}

func New[T any](capacity int) *RingBuffer[T] {
	if capacity <= 0 {
		capacity = 64
	}
	return &RingBuffer[T]{
		buf:    make([]T, capacity),
		maxCap: capacity,
	}
}

func (rb *RingBuffer[T]) Push(item T) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == rb.maxCap {
		return false
	}
	rb.buf[rb.head] = item
	rb.head = (rb.head + 1) % rb.maxCap
	rb.count++
	return true
}

func (rb *RingBuffer[T]) Pop() (T, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	item := rb.buf[rb.tail]
	rb.tail = (rb.tail + 1) % rb.maxCap
	rb.count--
	return item, true
}

func (rb *RingBuffer[T]) Peek() (T, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	return rb.buf[rb.tail], true
}

func (rb *RingBuffer[T]) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *RingBuffer[T]) Cap() int {
	return rb.maxCap
}

func (rb *RingBuffer[T]) Full() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count == rb.maxCap
}

func (rb *RingBuffer[T]) Empty() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count == 0
}

func (rb *RingBuffer[T]) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.head = 0
	rb.tail = 0
	rb.count = 0
}

func (rb *RingBuffer[T]) Items() []T {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	result := make([]T, rb.count)
	for i := 0; i < rb.count; i++ {
		result[i] = rb.buf[(rb.tail+i)%rb.maxCap]
	}
	return result
}

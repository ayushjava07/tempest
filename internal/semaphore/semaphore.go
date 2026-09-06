package semaphore

import (
	"context"
	"sync"
)

type Semaphore struct {
	mu    sync.Mutex
	tokens chan struct{}
}

func New(max int) *Semaphore {
	if max <= 0 {
		max = 1
	}
	return &Semaphore{
		tokens: make(chan struct{}, max),
	}
}

func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.tokens <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Semaphore) Release() {
	<-s.tokens
}

func (s *Semaphore) Available() int {
	return cap(s.tokens) - len(s.tokens)
}

func (s *Semaphore) Cap() int {
	return cap(s.tokens)
}

func (s *Semaphore) TryAcquire() bool {
	select {
	case s.tokens <- struct{}{}:
		return true
	default:
		return false
	}
}

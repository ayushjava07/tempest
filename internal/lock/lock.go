package lock

import (
	"context"
	"sync"
)

type Lock struct {
	mu         sync.Mutex
	holder     string
	held       bool
	waiters    []chan struct{}
}

func New() *Lock {
	return &Lock{
		waiters: make([]chan struct{}, 0),
	}
}

func (l *Lock) Lock(ctx context.Context) error {
	l.mu.Lock()
	if !l.held {
		l.held = true
		l.mu.Unlock()
		return nil
	}
	ch := make(chan struct{}, 1)
	l.waiters = append(l.waiters, ch)
	l.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		l.mu.Lock()
		for i, w := range l.waiters {
			if w == ch {
				l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
				break
			}
		}
		l.mu.Unlock()
		return ctx.Err()
	}
}

func (l *Lock) TryLock() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return false
	}
	l.held = true
	return true
}

func (l *Lock) Unlock() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return
	}
	if len(l.waiters) > 0 {
		ch := l.waiters[0]
		l.waiters = l.waiters[1:]
		ch <- struct{}{}
	} else {
		l.held = false
	}
}

func (l *Lock) IsHeld() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}

func (l *Lock) Waiters() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.waiters)
}

type RWLock struct {
	mu           sync.Mutex
	readers      int
	writerHeld   bool
	writerWaiters int
	readerWaiters []chan struct{}
	writerQueue  []chan struct{}
}

func NewRWLock() *RWLock {
	return &RWLock{
		readerWaiters: make([]chan struct{}, 0),
		writerQueue:   make([]chan struct{}, 0),
	}
}

func (rw *RWLock) RLock(ctx context.Context) error {
	rw.mu.Lock()
	if !rw.writerHeld && rw.writerWaiters == 0 {
		rw.readers++
		rw.mu.Unlock()
		return nil
	}
	ch := make(chan struct{}, 1)
	rw.readerWaiters = append(rw.readerWaiters, ch)
	rw.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		rw.mu.Lock()
		for i, w := range rw.readerWaiters {
			if w == ch {
				rw.readerWaiters = append(rw.readerWaiters[:i], rw.readerWaiters[i+1:]...)
				break
			}
		}
		rw.mu.Unlock()
		return ctx.Err()
	}
}

func (rw *RWLock) RUnlock() {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.readers--
	if rw.readers == 0 && len(rw.writerQueue) > 0 {
		ch := rw.writerQueue[0]
		rw.writerQueue = rw.writerQueue[1:]
		rw.writerHeld = true
		rw.writerWaiters--
		ch <- struct{}{}
	}
}

func (rw *RWLock) Lock(ctx context.Context) error {
	rw.mu.Lock()
	if rw.readers == 0 && !rw.writerHeld {
		rw.writerHeld = true
		rw.mu.Unlock()
		return nil
	}
	ch := make(chan struct{}, 1)
	rw.writerQueue = append(rw.writerQueue, ch)
	rw.writerWaiters++
	rw.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		rw.mu.Lock()
		for i, w := range rw.writerQueue {
			if w == ch {
				rw.writerQueue = append(rw.writerQueue[:i], rw.writerQueue[i+1:]...)
				break
			}
		}
		rw.writerWaiters--
		rw.mu.Unlock()
		return ctx.Err()
	}
}

func (rw *RWLock) Unlock() {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if !rw.writerHeld {
		return
	}
	rw.writerHeld = false
	if len(rw.readerWaiters) > 0 {
		for len(rw.readerWaiters) > 0 {
			ch := rw.readerWaiters[0]
			rw.readerWaiters = rw.readerWaiters[1:]
			rw.readers++
			ch <- struct{}{}
		}
	} else if len(rw.writerQueue) > 0 {
		ch := rw.writerQueue[0]
		rw.writerQueue = rw.writerQueue[1:]
		rw.writerHeld = true
		rw.writerWaiters--
		ch <- struct{}{}
	}
}

func (rw *RWLock) TryRLock() bool {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.writerHeld || rw.writerWaiters > 0 {
		return false
	}
	rw.readers++
	return true
}

func (rw *RWLock) TryLock() bool {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.readers > 0 || rw.writerHeld {
		return false
	}
	rw.writerHeld = true
	return true
}
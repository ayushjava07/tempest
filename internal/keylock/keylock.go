package keylock

import (
	"sync"
	"time"
)

type entry struct {
	deadline time.Time
	timer    *time.Timer
}

type KeyLock struct {
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
	entries map[string]*entry
	timeout time.Duration
}

func New(timeout time.Duration) *KeyLock {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &KeyLock{
		locks:   make(map[string]*sync.Mutex),
		entries: make(map[string]*entry),
		timeout: timeout,
	}
}

func (kl *KeyLock) Lock(key string) {
	kl.mu.Lock()
	if kl.locks[key] == nil {
		kl.locks[key] = &sync.Mutex{}
	}
	mu := kl.locks[key]
	kl.mu.Unlock()
	mu.Lock()
}

func (kl *KeyLock) Unlock(key string) {
	kl.mu.Lock()
	mu, ok := kl.locks[key]
	kl.mu.Unlock()
	if ok {
		mu.Unlock()
	}
}

func (kl *KeyLock) TryLock(key string) bool {
	kl.mu.Lock()
	if kl.locks[key] == nil {
		kl.locks[key] = &sync.Mutex{}
	}
	mu := kl.locks[key]
	kl.mu.Unlock()
	return mu.TryLock()
}

func (kl *KeyLock) LockWithTimeout(key string, timeout time.Duration) bool {
	kl.mu.Lock()
	if kl.locks[key] == nil {
		kl.locks[key] = &sync.Mutex{}
	}
	mu := kl.locks[key]
	kl.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ch := make(chan struct{})
	go func() {
		mu.Lock()
		close(ch)
	}()
	select {
	case <-ch:
		return true
	case <-timer.C:
		return false
	}
}

func (kl *KeyLock) LockedKeys() []string {
	kl.mu.Lock()
	defer kl.mu.Unlock()
	var keys []string
	for k := range kl.locks {
		keys = append(keys, k)
	}
	return keys
}

package keylock

import (
	"sync"
	"testing"
	"time"
)

func TestKeyLock_Basic(t *testing.T) {
	kl := New(30 * time.Second)
	kl.Lock("a")
	kl.Unlock("a")
}

func TestKeyLock_Concurrent(t *testing.T) {
	kl := New(30 * time.Second)
	var counter int
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kl.Lock("shared")
			counter++
			kl.Unlock("shared")
		}()
	}
	wg.Wait()
	if counter != 100 {
		t.Errorf("expected 100, got %d", counter)
	}
}

func TestKeyLock_DifferentKeys(t *testing.T) {
	kl := New(30 * time.Second)
	kl.Lock("a")
	if !kl.TryLock("b") {
		t.Error("different key should not block")
	}
	kl.Unlock("a")
	kl.Unlock("b")
}

func TestKeyLock_TryLock(t *testing.T) {
	kl := New(30 * time.Second)
	if !kl.TryLock("a") {
		t.Error("expected success")
	}
	if kl.TryLock("a") {
		t.Error("expected contention")
	}
	kl.Unlock("a")
}

func TestKeyLock_LockedKeys(t *testing.T) {
	kl := New(30 * time.Second)
	kl.Lock("a")
	kl.Lock("b")
	keys := kl.LockedKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2, got %d", len(keys))
	}
	kl.Unlock("a")
	kl.Unlock("b")
}

func TestKeyLock_LockWithTimeout(t *testing.T) {
	kl := New(30 * time.Second)
	kl.Lock("x")
	go func() {
		time.Sleep(50 * time.Millisecond)
		kl.Unlock("x")
	}()
	if !kl.LockWithTimeout("x", 200*time.Millisecond) {
		t.Error("expected to acquire")
	}
}

func TestKeyLock_LockWithTimeout_Expiry(t *testing.T) {
	kl := New(30 * time.Second)
	kl.Lock("x")
	defer kl.Unlock("x")
	if kl.LockWithTimeout("x", 50*time.Millisecond) {
		t.Error("expected timeout")
	}
}

func TestKeyLock_EmptyKeys(t *testing.T) {
	kl := New(30 * time.Second)
	keys := kl.LockedKeys()
	if len(keys) != 0 {
		t.Error("expected no keys")
	}
}

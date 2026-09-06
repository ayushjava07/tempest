package ratepool

import (
	"sync"
	"testing"
	"time"
)

func TestPool_Allow(t *testing.T) {
	p := New(time.Second, 5)
	defer p.Stop()
	if !p.Allow("key") {
		t.Error("expected allow")
	}
}

func TestPool_AllowN(t *testing.T) {
	p := New(time.Second, 3)
	defer p.Stop()
	if !p.AllowN("k", 3) {
		t.Error("expected allow 3")
	}
	if p.AllowN("k", 1) {
		t.Error("expected deny after exhausting")
	}
}

func TestPool_Refill(t *testing.T) {
	p := New(50*time.Millisecond, 2)
	defer p.Stop()
	p.AllowN("k", 2)
	if p.Allow("k") {
		t.Error("expected deny")
	}
	time.Sleep(60 * time.Millisecond)
	if !p.Allow("k") {
		t.Error("expected allow after refill")
	}
}

func TestPool_Tokens(t *testing.T) {
	p := New(time.Second, 5)
	defer p.Stop()
	p.AllowN("k", 2)
	if p.Tokens("k") != 3 {
		t.Errorf("expected 3, got %d", p.Tokens("k"))
	}
}

func TestPool_Tokens_MissingKey(t *testing.T) {
	p := New(time.Second, 5)
	defer p.Stop()
	if p.Tokens("new") != 5 {
		t.Error("expected max tokens for new key")
	}
}

func TestPool_Reset(t *testing.T) {
	p := New(time.Second, 5)
	defer p.Stop()
	p.Allow("k")
	p.Reset("k")
	if p.Tokens("k") != 5 {
		t.Error("expected reset tokens")
	}
}

func TestPool_Size(t *testing.T) {
	p := New(time.Second, 5)
	defer p.Stop()
	p.Allow("a")
	p.Allow("b")
	if p.Size() != 2 {
		t.Errorf("expected 2, got %d", p.Size())
	}
}

func TestPool_Concurrent(t *testing.T) {
	p := New(time.Second, 100)
	defer p.Stop()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Allow("shared")
		}()
	}
	wg.Wait()
}

func TestPool_DifferentKeys(t *testing.T) {
	p := New(time.Second, 1)
	defer p.Stop()
	if !p.Allow("a") {
		t.Error("expected allow a")
	}
	if !p.Allow("b") {
		t.Error("expected allow b (different key)")
	}
}

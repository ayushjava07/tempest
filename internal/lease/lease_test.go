package lease

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestManager_Acquire(t *testing.T) {
	m := NewManager(func() string { return "token" })
	l, err := m.Acquire(context.Background(), "holder1", "resource1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if l.Holder != "holder1" {
		t.Error("holder mismatch")
	}
}

func TestManager_Acquire_Conflict(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", time.Hour)
	_, err := m.Acquire(context.Background(), "holder2", "resource1", time.Hour)
	if err == nil {
		t.Error("expected conflict")
	}
}

func TestManager_Renew(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", 50*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if err := m.Renew(context.Background(), "holder1", "resource1", time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestManager_Renew_WrongHolder(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", time.Hour)
	err := m.Renew(context.Background(), "holder2", "resource1", time.Hour)
	if err == nil {
		t.Error("expected conflict")
	}
}

func TestManager_Release(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", time.Hour)
	if err := m.Release(context.Background(), "holder1", "resource1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("resource1"); ok {
		t.Error("expected released")
	}
}

func TestManager_Release_WrongHolder(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", time.Hour)
	err := m.Release(context.Background(), "holder2", "resource1")
	if err == nil {
		t.Error("expected conflict")
	}
}

func TestManager_Get(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", time.Hour)
	l, ok := m.Get("resource1")
	if !ok || l.Holder != "holder1" {
		t.Error("expected lease")
	}
}

func TestManager_Get_Expired(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "holder1", "resource1", 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	l, ok := m.Get("resource1")
	if ok {
		t.Error("expected expired")
	}
	if l != nil {
		t.Error("expected nil")
	}
}

func TestManager_Expired(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "a", "r1", 10*time.Millisecond)
	m.Acquire(context.Background(), "b", "r2", time.Hour)
	time.Sleep(20 * time.Millisecond)
	expired := m.Expired()
	if len(expired) != 1 {
		t.Errorf("expected 1 expired, got %d", len(expired))
	}
}

func TestManager_All(t *testing.T) {
	m := NewManager(func() string { return "token" })
	m.Acquire(context.Background(), "a", "r1", time.Hour)
	m.Acquire(context.Background(), "b", "r2", time.Hour)
	all := m.All()
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestManager_Concurrent(t *testing.T) {
	m := NewManager(func() string { return "token" })
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Acquire(context.Background(), "holder", "resource-"+string(rune(n)), time.Hour)
		}(i)
	}
	wg.Wait()
	if len(m.All()) != 100 {
		t.Errorf("expected 100, got %d", len(m.All()))
	}
}
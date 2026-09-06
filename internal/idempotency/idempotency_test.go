package idempotency

import (
	"testing"
	"time"
)

func TestStore_CheckMiss(t *testing.T) {
	s := NewStore(time.Minute)
	_, ok := s.Check("run-1", "default")
	if ok {
		t.Error("expected miss")
	}
}

func TestStore_MarkCheckHit(t *testing.T) {
	s := NewStore(time.Minute)
	s.Mark("run-1", "default", "result", "")
	entry, ok := s.Check("run-1", "default")
	if !ok {
		t.Fatal("expected hit")
	}
	if entry.Result != "result" {
		t.Errorf("expected result, got %v", entry.Result)
	}
}

func TestStore_DifferentNamespaces(t *testing.T) {
	s := NewStore(time.Minute)
	s.Mark("run-1", "ns1", "a", "")
	s.Mark("run-1", "ns2", "b", "")
	e1, _ := s.Check("run-1", "ns1")
	e2, _ := s.Check("run-1", "ns2")
	if e1.Result != "a" || e2.Result != "b" {
		t.Error("expected different results per namespace")
	}
}

func TestStore_Expiry(t *testing.T) {
	s := NewStore(50 * time.Millisecond)
	s.Mark("run-1", "default", "result", "")
	time.Sleep(100 * time.Millisecond)
	_, ok := s.Check("run-1", "default")
	if ok {
		t.Error("expected expired entry to be missed")
	}
}

func TestStore_Remove(t *testing.T) {
	s := NewStore(time.Minute)
	s.Mark("run-1", "default", "result", "")
	s.Remove("run-1", "default")
	_, ok := s.Check("run-1", "default")
	if ok {
		t.Error("expected miss after remove")
	}
}

func TestStore_Size(t *testing.T) {
	s := NewStore(time.Minute)
	s.Mark("a", "default", "1", "")
	s.Mark("b", "default", "2", "")
	if s.Size() != 2 {
		t.Errorf("expected 2, got %d", s.Size())
	}
}

func TestStore_Clear(t *testing.T) {
	s := NewStore(time.Minute)
	s.Mark("a", "default", "1", "")
	s.Clear()
	if s.Size() != 0 {
		t.Error("expected empty store")
	}
}

func TestStore_ErrorEntry(t *testing.T) {
	s := NewStore(time.Minute)
	s.Mark("run-1", "default", nil, "timeout error")
	entry, _ := s.Check("run-1", "default")
	if entry.Error != "timeout error" {
		t.Errorf("expected timeout error, got %s", entry.Error)
	}
}

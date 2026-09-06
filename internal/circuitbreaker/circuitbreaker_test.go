package circuitbreaker

import (
	"testing"
	"time"
)

func TestBreaker_Closed(t *testing.T) {
	b := New(3, 2, time.Second)
	if b.State() != StateClosed {
		t.Error("expected closed state")
	}
	if !b.Allow() {
		t.Error("expected allowed when closed")
	}
}

func TestBreaker_Open(t *testing.T) {
	b := New(3, 2, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Error("expected open state")
	}
	if b.Allow() {
		t.Error("expected denied when open")
	}
}

func TestBreaker_HalfOpen(t *testing.T) {
	b := New(3, 2, 50*time.Millisecond)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordFailure()
	time.Sleep(100 * time.Millisecond)
	if b.State() != StateHalfOpen {
		t.Error("expected half-open state")
	}
	if !b.Allow() {
		t.Error("expected allowed in half-open")
	}
}

func TestBreaker_Recovery(t *testing.T) {
	b := New(3, 2, 50*time.Millisecond)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordFailure()
	time.Sleep(100 * time.Millisecond)
	b.Allow()
	b.RecordSuccess()
	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Error("expected recovered to closed")
	}
}

func TestBreaker_HalfOpenFailure(t *testing.T) {
	b := New(3, 2, 50*time.Millisecond)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordFailure()
	time.Sleep(100 * time.Millisecond)
	b.Allow()
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Error("expected back to open")
	}
}

func TestBreaker_Reset(t *testing.T) {
	b := New(3, 2, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordFailure()
	b.Reset()
	if b.State() != StateClosed {
		t.Error("expected closed after reset")
	}
}

func TestBreaker_FailureCount(t *testing.T) {
	b := New(3, 2, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	if b.FailureCount() != 2 {
		t.Errorf("expected 2, got %d", b.FailureCount())
	}
}

func TestBreaker_StateTransition(t *testing.T) {
	b := New(2, 1, time.Second)
	if b.State().String() != "closed" {
		t.Error("expected closed")
	}
	b.RecordFailure()
	if b.State().String() != "closed" {
		t.Error("expected still closed")
	}
	b.RecordFailure()
	if b.State().String() != "open" {
		t.Error("expected open")
	}
}

func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := New(3, 2, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess()
	if b.FailureCount() != 0 {
		t.Error("expected failure count reset on success")
	}
}

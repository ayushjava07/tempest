package quota

import (
	"sync"
	"testing"
	"time"
)

func TestQuota_Allow(t *testing.T) {
	q := New(10, time.Hour, 10)
	if !q.Allow(5) {
		t.Error("expected allow 5")
	}
	if !q.Allow(5) {
		t.Error("expected allow 5")
	}
	if q.Allow(1) {
		t.Error("expected deny, limit reached")
	}
}

func TestQuota_Remaining(t *testing.T) {
	q := New(10, time.Hour, 10)
	q.Allow(3)
	if q.Remaining() != 7 {
		t.Errorf("expected 7, got %d", q.Remaining())
	}
}

func TestQuota_Used(t *testing.T) {
	q := New(10, time.Hour, 10)
	q.Allow(4)
	if q.Used() != 4 {
		t.Errorf("expected 4, got %d", q.Used())
	}
}

func TestQuota_Refill(t *testing.T) {
	q := New(10, 50*time.Millisecond, 10)
	q.Allow(10)
	time.Sleep(60 * time.Millisecond)
	if !q.Allow(5) {
		t.Error("expected allow after refill")
	}
}

func TestQuota_Reset(t *testing.T) {
	q := New(10, time.Hour, 10)
	q.Allow(10)
	q.Reset()
	if q.Remaining() != 10 {
		t.Error("expected 10 after reset")
	}
}

func TestQuota_Concurrent(t *testing.T) {
	q := New(100, time.Hour, 100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Allow(2)
		}()
	}
	wg.Wait()
	if q.Used() != 100 {
		t.Errorf("expected 100, got %d", q.Used())
	}
}

func TestQuota_Limit(t *testing.T) {
	q := New(42, time.Hour, 10)
	if q.Limit() != 42 {
		t.Errorf("expected 42, got %d", q.Limit())
	}
}
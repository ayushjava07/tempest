package throttler

import (
	"testing"
	"time"
)

func TestCompressedTimestampRing(t *testing.T) {
	ring := NewCompressedTimestampRing(10)
	now := time.Now()

	// Push 5 timestamps spaced by 10ms
	for i := 0; i < 5; i++ {
		ring.Push(now.Add(time.Duration(i*10) * time.Millisecond))
	}

	if ring.Len() != 5 {
		t.Fatalf("expected 5 items, got %d", ring.Len())
	}

	// Count within 25ms window from last timestamp (+40ms)
	evalTime := now.Add(40 * time.Millisecond)
	count, _ := ring.CountWithinWindow(evalTime, 25*time.Millisecond)
	// 40 - 25 = 15ms cutoff -> timestamps at 20ms, 30ms, 40ms = 3 items
	if count != 3 {
		t.Fatalf("expected 3 items within 25ms window, got %d", count)
	}
}

func BenchmarkCompressedTimestampRingPush(b *testing.B) {
	ring := NewCompressedTimestampRing(10000)
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.Push(now)
	}
}

func BenchmarkSlidingLogAllow(b *testing.B) {
	th := NewSlidingLogThrottler()
	_ = th.SetQuota(ThrottleConfig{
		QuotaKey:  "bench-key",
		RateLimit: 1000000,
		Window:    time.Second,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = th.Allow("bench-key")
	}
}

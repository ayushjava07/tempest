package throttler

import (
	"testing"
	"time"
)

func TestAutoTunerReductionAndRecovery(t *testing.T) {
	baseThrottler := NewSlidingLogThrottler()
	tuner := NewAutoTuner(baseThrottler, 5, 0.5)
	tuner.probeInterval = 3 // Fast probe for unit testing

	cfg := ThrottleConfig{
		QuotaKey:  "external-ai-api",
		RateLimit: 100,
		Window:    time.Minute,
	}

	if err := tuner.RegisterBaseQuota(cfg); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if tuner.EffectiveRate("external-ai-api") != 100 {
		t.Fatalf("expected initial rate 100, got %d", tuner.EffectiveRate("external-ai-api"))
	}

	// 1. First 429 received -> rate halved to 50
	tuner.RecordFailure("external-ai-api", 429, 20*time.Millisecond)
	if tuner.EffectiveRate("external-ai-api") != 50 {
		t.Fatalf("expected rate reduced to 50, got %d", tuner.EffectiveRate("external-ai-api"))
	}

	if !tuner.IsCoolingOff("external-ai-api") {
		t.Fatalf("expected cooling off to be true")
	}

	time.Sleep(25 * time.Millisecond)
	if tuner.IsCoolingOff("external-ai-api") {
		t.Fatalf("expected cooling off to expire")
	}

	// 2. Second 429 -> halved to 25
	tuner.RecordFailure("external-ai-api", 429, 0)
	if tuner.EffectiveRate("external-ai-api") != 25 {
		t.Fatalf("expected rate 25, got %d", tuner.EffectiveRate("external-ai-api"))
	}

	// 3. Repeated failures must not drop below minRate=5
	for i := 0; i < 5; i++ {
		tuner.RecordFailure("external-ai-api", 429, 0)
	}
	if tuner.EffectiveRate("external-ai-api") != 5 {
		t.Fatalf("expected rate floored at minRate 5, got %d", tuner.EffectiveRate("external-ai-api"))
	}

	// 4. Recovery: 3 successes -> rate increases to 6
	for i := 0; i < 3; i++ {
		tuner.RecordSuccess("external-ai-api")
	}
	if tuner.EffectiveRate("external-ai-api") != 6 {
		t.Fatalf("expected rate incremented to 6, got %d", tuner.EffectiveRate("external-ai-api"))
	}

	// 3 more successes -> rate increases to 7
	for i := 0; i < 3; i++ {
		tuner.RecordSuccess("external-ai-api")
	}
	if tuner.EffectiveRate("external-ai-api") != 7 {
		t.Fatalf("expected rate incremented to 7, got %d", tuner.EffectiveRate("external-ai-api"))
	}
}

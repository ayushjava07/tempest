package cronengine

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCron_ParseExpression(t *testing.T) {
	tests := []struct {
		spec    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"*/15 9-17 * * 1-5", false},
		{"0 0 1 1 *", false},
		{"59 23 31 12 6", false},
		{"* * * *", true},     // 4 fields
		{"* * * * * *", true}, // 6 fields
		{"60 * * * *", true},  // minute out of bounds
		{"* 24 * * *", true},  // hour out of bounds
		{"* * 0 * *", true},   // dom out of bounds (1-31)
		{"* * 32 * *", true},  // dom out of bounds
		{"* * * 13 *", true},  // month out of bounds
		{"* * * * 7", true},   // dow out of bounds (0-6)
		{"*/0 * * * *", true}, // step 0
		{"5-2 * * * *", true}, // inverted range
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			_, err := ParseExpression(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseExpression(%q) error = %v, wantErr = %v", tt.spec, err, tt.wantErr)
			}
		})
	}
}

func TestCron_NextCalculation(t *testing.T) {
	expr, err := ParseExpression("30 14 * * *") // 14:30 everyday
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	base := time.Date(2026, time.January, 15, 10, 0, 0, 0, time.UTC)
	next := expr.Next(base, time.UTC)

	expected := time.Date(2026, time.January, 15, 14, 30, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected next run at %v, got %v", expected, next)
	}

	// Base after 14:30
	baseLater := time.Date(2026, time.January, 15, 15, 0, 0, 0, time.UTC)
	nextDay := expr.Next(baseLater, time.UTC)
	expectedNextDay := time.Date(2026, time.January, 16, 14, 30, 0, 0, time.UTC)
	if !nextDay.Equal(expectedNextDay) {
		t.Fatalf("expected next day run at %v, got %v", expectedNextDay, nextDay)
	}
}

func TestCron_TriggerNowAndExecutionRecord(t *testing.T) {
	eng := New(DefaultEngineConfig())

	var executed atomic.Int32
	err := eng.AddSchedule(
		"sched-1",
		"wf-1",
		"0 0 1 1 *", // Annually
		time.UTC,
		MissedFireSkip,
		OverlapAllow,
		func(ctx context.Context, scheduledAt time.Time) error {
			executed.Add(1)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("AddSchedule failed: %v", err)
	}

	err = eng.TriggerNow("sched-1")
	if err != nil {
		t.Fatalf("TriggerNow failed: %v", err)
	}

	// Allow execution to finish
	time.Sleep(50 * time.Millisecond)

	if executed.Load() != 1 {
		t.Fatalf("expected 1 execution, got %d", executed.Load())
	}

	history := eng.GetHistory("sched-1")
	if len(history) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(history))
	}
	if !history[0].Success {
		t.Fatalf("expected execution success, got err: %v", history[0].Err)
	}
}

func TestCron_OverlapForbid(t *testing.T) {
	eng := New(DefaultEngineConfig())

	inFlight := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32

	err := eng.AddSchedule(
		"forbid-sched",
		"wf-overlap",
		"0 0 1 1 *",
		time.UTC,
		MissedFireSkip,
		OverlapForbid,
		func(ctx context.Context, scheduledAt time.Time) error {
			runs.Add(1)
			close(inFlight)
			<-release
			return nil
		},
	)
	if err != nil {
		t.Fatalf("failed to add schedule: %v", err)
	}

	// Trigger first run
	_ = eng.TriggerNow("forbid-sched")
	<-inFlight

	// Attempt second run while first is still running
	_ = eng.TriggerNow("forbid-sched")

	// Release first run
	close(release)
	time.Sleep(50 * time.Millisecond)

	if runs.Load() != 1 {
		t.Fatalf("expected exactly 1 run under OverlapForbid, got %d", runs.Load())
	}

	history := eng.GetHistory("forbid-sched")
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries (one success, one skipped), got %d", len(history))
	}

	var hasSuccess, hasSkipped bool
	for _, rec := range history {
		if rec.Success {
			hasSuccess = true
		}
		if rec.Err != nil && strings.Contains(rec.Err.Error(), "OverlapForbid") {
			hasSkipped = true
		}
	}
	if !hasSuccess || !hasSkipped {
		t.Fatalf("expected one success and one OverlapForbid error, got history: %+v", history)
	}
}

func TestCron_PauseAndResume(t *testing.T) {
	eng := New(DefaultEngineConfig())

	err := eng.AddSchedule(
		"p-sched",
		"wf-p",
		"* * * * *",
		time.UTC,
		MissedFireSkip,
		OverlapAllow,
		func(ctx context.Context, scheduledAt time.Time) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("AddSchedule failed: %v", err)
	}

	if err := eng.PauseSchedule("p-sched"); err != nil {
		t.Fatalf("PauseSchedule failed: %v", err)
	}

	s, _ := eng.GetSchedule("p-sched")
	if !s.IsPaused {
		t.Fatalf("expected schedule to be paused")
	}

	if err := eng.ResumeSchedule("p-sched"); err != nil {
		t.Fatalf("ResumeSchedule failed: %v", err)
	}

	s, _ = eng.GetSchedule("p-sched")
	if s.IsPaused {
		t.Fatalf("expected schedule to be resumed")
	}

	if err := eng.RemoveSchedule("p-sched"); err != nil {
		t.Fatalf("RemoveSchedule failed: %v", err)
	}

	_, err = eng.GetSchedule("p-sched")
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound, got %v", err)
	}
}

func TestCron_LifecycleStartStop(t *testing.T) {
	eng := New(DefaultEngineConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stopping should be clean and not leak goroutines
	eng.Stop()
}

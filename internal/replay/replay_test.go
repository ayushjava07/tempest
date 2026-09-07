package replay

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeterministicSequentialReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec := NewRecorder("run-1001", "wf-sequential")

	// Simulate recording a multi-step sequential workflow run
	rec.Record(EventWorkflowStarted, "", map[string]string{"trigger": "manual"})
	rec.Record(EventStepScheduled, "step-1", map[string]string{"action": "fetch"})
	rec.Record(EventStepStarted, "step-1", nil)
	rec.Record(EventStepCompleted, "step-1", map[string]string{"items": "42"})
	rec.Record(EventStepScheduled, "step-2", map[string]string{"action": "process"})
	rec.Record(EventStepStarted, "step-2", nil)
	rec.Record(EventStepCompleted, "step-2", map[string]string{"processed": "true"})
	rec.Record(EventWorkflowCompleted, "", map[string]string{"status": "SUCCESS"})

	history := rec.Snapshot()
	if len(history.Events) != 8 {
		t.Fatalf("expected 8 events, got %d", len(history.Events))
	}

	// Initialize replayer and verify exact step-by-step match
	replayer := NewReplayer(history)

	stepsExpected := []struct {
		evtType EventType
		stepID  string
	}{
		{EventWorkflowStarted, ""},
		{EventStepScheduled, "step-1"},
		{EventStepStarted, "step-1"},
		{EventStepCompleted, "step-1"},
		{EventStepScheduled, "step-2"},
		{EventStepStarted, "step-2"},
		{EventStepCompleted, "step-2"},
		{EventWorkflowCompleted, ""},
	}

	for i, exp := range stepsExpected {
		evt, err := replayer.MatchStep(exp.evtType, exp.stepID)
		if err != nil {
			t.Fatalf("at index %d: unexpected MatchStep error: %v", i, err)
		}
		if evt.Type != exp.evtType || evt.StepID != exp.stepID {
			t.Fatalf("mismatch at %d: got type=%s step=%s", i, evt.Type, evt.StepID)
		}
	}

	if !replayer.IsComplete() {
		t.Fatalf("expected replayer to be complete, remaining=%d", replayer.RemainingEvents())
	}

	// Verify extra step past history returns ErrHistoryExhausted
	_, err := replayer.MatchStep(EventWorkflowCompleted, "")
	if !errors.Is(err, ErrHistoryExhausted) {
		t.Fatalf("expected ErrHistoryExhausted, got %v", err)
	}

	// Test Replay runner method
	replayer.Reset()
	if replayer.Cursor() != 0 {
		t.Fatalf("expected cursor to reset to 0, got %d", replayer.Cursor())
	}

	var visitedEvents []HistoryEvent
	report, err := replayer.Replay(ctx, func(c context.Context, evt HistoryEvent) error {
		visitedEvents = append(visitedEvents, evt)
		return nil
	})
	if err != nil {
		t.Fatalf("replayer.Replay failed: %v", err)
	}

	if !report.Completed {
		t.Fatalf("expected replay report to be completed")
	}
	if report.MatchedEvents != 8 {
		t.Fatalf("expected 8 matched events, got %d", report.MatchedEvents)
	}
	if len(visitedEvents) != 8 {
		t.Fatalf("expected 8 visited events, got %d", len(visitedEvents))
	}
}

func TestReplayerPeekAndCursor(t *testing.T) {
	rec := NewRecorder("run-1002", "wf-peek")
	rec.Record(EventWorkflowStarted, "", nil)
	rec.Record(EventStepScheduled, "step-A", nil)

	replayer := NewReplayer(rec.Snapshot())

	evt, ok := replayer.Peek()
	if !ok || evt.Type != EventWorkflowStarted {
		t.Fatalf("expected peek at WorkflowStarted, got %v (ok=%v)", evt, ok)
	}
	if replayer.Cursor() != 0 {
		t.Fatalf("peek should not advance cursor, cursor=%d", replayer.Cursor())
	}

	_, err := replayer.MatchStep(EventWorkflowStarted, "")
	if err != nil {
		t.Fatalf("match step failed: %v", err)
	}
	if replayer.Cursor() != 1 {
		t.Fatalf("expected cursor=1, got %d", replayer.Cursor())
	}

	evt, ok = replayer.Peek()
	if !ok || evt.StepID != "step-A" {
		t.Fatalf("expected peek at step-A, got %v", evt)
	}
}

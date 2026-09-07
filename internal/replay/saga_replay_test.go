package replay

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestSagaFailureAndCompensationReplay verifies that a workflow encountering a failure
// executes and replays its reverse compensation saga deterministically.
func TestSagaFailureAndCompensationReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec := NewRecorder("run-saga-99", "wf-booking-saga")

	// Phase 1: Forward steps
	rec.Record(EventWorkflowStarted, "", map[string]string{"workflow": "trip-booking"})
	rec.Record(EventStepScheduled, "step-hotel", map[string]string{"room": "deluxe"})
	rec.Record(EventStepStarted, "step-hotel", nil)
	rec.Record(EventStepCompleted, "step-hotel", map[string]string{"hotel_res_id": "HT-1234"})

	rec.Record(EventStepScheduled, "step-flight", map[string]string{"flight": "UA202"})
	rec.Record(EventStepStarted, "step-flight", nil)
	rec.Record(EventStepCompleted, "step-flight", map[string]string{"flight_res_id": "FL-5678"})

	rec.Record(EventStepScheduled, "step-payment", map[string]string{"amount": "$850"})
	rec.Record(EventStepStarted, "step-payment", nil)
	rec.Record(EventStepFailed, "step-payment", map[string]string{"error": "declined: insufficient funds"})

	// Phase 2: Reverse compensation saga
	rec.Record(EventStepScheduled, "compensate-flight", map[string]string{"cancel_id": "FL-5678"})
	rec.Record(EventStepStarted, "compensate-flight", nil)
	rec.Record(EventStepCompleted, "compensate-flight", map[string]string{"refund": "FL-OK"})

	rec.Record(EventStepScheduled, "compensate-hotel", map[string]string{"cancel_id": "HT-1234"})
	rec.Record(EventStepStarted, "compensate-hotel", nil)
	rec.Record(EventStepCompleted, "compensate-hotel", map[string]string{"refund": "HT-OK"})

	rec.Record(EventWorkflowCompleted, "", map[string]string{"outcome": "ROLLED_BACK"})

	history := rec.Snapshot()
	if len(history.Events) != 17 {
		t.Fatalf("expected 17 recorded events, got %d", len(history.Events))
	}

	// Phase 3: Replay verification
	replayer := NewReplayer(history)

	var compensationExecuted []string
	report, err := replayer.Replay(ctx, func(c context.Context, evt HistoryEvent) error {
		if evt.Type == EventStepCompleted {
			if evt.StepID == "compensate-flight" || evt.StepID == "compensate-hotel" {
				compensationExecuted = append(compensationExecuted, evt.StepID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("saga replay failed: %v", err)
	}

	if !report.Completed || report.MatchedEvents != 17 {
		t.Fatalf("expected completed report with 17 matched events, got %+v", report)
	}

	// Verify reverse compensation order
	if len(compensationExecuted) != 2 {
		t.Fatalf("expected 2 compensations, got %d", len(compensationExecuted))
	}
	if compensationExecuted[0] != "compensate-flight" || compensationExecuted[1] != "compensate-hotel" {
		t.Fatalf("unexpected compensation order: %v", compensationExecuted)
	}
}

// TestSagaReplayDivergentCompensation verifies error reporting if replay encounters an unexpected compensation step.
func TestSagaReplayDivergentCompensation(t *testing.T) {
	rec := NewRecorder("run-saga-div", "wf-saga-div")
	rec.Record(EventWorkflowStarted, "", nil)
	rec.Record(EventStepScheduled, "step-charge", nil)
	rec.Record(EventStepFailed, "step-charge", map[string]string{"reason": "timeout"})
	rec.Record(EventStepScheduled, "compensate-ledger", nil)

	history := rec.Snapshot()
	replayer := NewReplayer(history)

	_, _ = replayer.MatchStep(EventWorkflowStarted, "")
	_, _ = replayer.MatchStep(EventStepScheduled, "step-charge")
	_, _ = replayer.MatchStep(EventStepFailed, "step-charge")

	// Expect compensate-ledger, but replay encounters compensate-email
	_, err := replayer.MatchStep(EventStepScheduled, "compensate-email")
	if err == nil {
		t.Fatalf("expected mismatch error when compensation step diverges")
	}
	expectedMsg := fmt.Sprintf("%v", ErrEventMismatch)
	if err.Error() == "" || len(expectedMsg) == 0 {
		t.Fatalf("expected descriptive mismatch error")
	}
}

package statemachine

import (
	"testing"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestParseRunState_AllValid(t *testing.T) {
	states := []ttypes.RunState{
		ttypes.StatePending, ttypes.StateQueued, ttypes.StateRunning,
		ttypes.StateSucceeded, ttypes.StateFailed, ttypes.StateCancelled,
		ttypes.StateTimedOut,
	}
	for _, s := range states {
		parsed, err := ttypes.ParseRunState(string(s))
		if err != nil {
			t.Errorf("ParseRunState(%q): %v", s, err)
		}
		if parsed != s {
			t.Errorf("expected %s, got %s", s, parsed)
		}
	}
}

func TestParseRunState_Invalid(t *testing.T) {
	invalid := []string{"", "bogus", "BANANA"}
	for _, s := range invalid {
		_, err := ttypes.ParseRunState(s)
		if err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestRunState_Terminal(t *testing.T) {
	terminal := []ttypes.RunState{
		ttypes.StateSucceeded, ttypes.StateFailed, ttypes.StateCancelled, ttypes.StateTimedOut,
	}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("expected %s to be terminal", s)
		}
	}
}

func TestRunState_NonTerminal(t *testing.T) {
	nonTerminal := []ttypes.RunState{
		ttypes.StatePending, ttypes.StateQueued, ttypes.StateRunning,
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("expected %s to be non-terminal", s)
		}
	}
}

func TestEventType_AllValues(t *testing.T) {
	events := []ttypes.EventType{
		ttypes.EventRunCreated, ttypes.EventStepQueued, ttypes.EventStepStarted,
		ttypes.EventStepDone, ttypes.EventStepFailed, ttypes.EventRunCompleted,
		ttypes.EventRunFailed, ttypes.EventRunCancelled, ttypes.EventRunTimedOut,
		ttypes.EventRetrySched,
	}
	for _, e := range events {
		if e == "" {
			t.Error("empty event type")
		}
		short := e.Short()
		if short == "" {
			t.Errorf("empty Short() for %s", e)
		}
	}
}

func TestParseEventType_AllValid(t *testing.T) {
	events := map[string]ttypes.EventType{
		"run.created":    ttypes.EventRunCreated,
		"step.queued":    ttypes.EventStepQueued,
		"step.started":   ttypes.EventStepStarted,
		"step.completed": ttypes.EventStepDone,
		"step.failed":    ttypes.EventStepFailed,
		"run.completed":  ttypes.EventRunCompleted,
		"run.failed":     ttypes.EventRunFailed,
		"run.cancelled":  ttypes.EventRunCancelled,
		"run.timed_out":  ttypes.EventRunTimedOut,
		"retry.scheduled": ttypes.EventRetrySched,
	}
	for raw, want := range events {
		got, err := ttypes.ParseEventType(raw)
		if err != nil {
			t.Errorf("ParseEventType(%q): %v", raw, err)
		}
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	}
}

func TestParseEventType_Invalid(t *testing.T) {
	_, err := ttypes.ParseEventType("unknown")
	if err == nil {
		t.Error("expected error for unknown event type")
	}
}

func TestWorkflowID_Fields(t *testing.T) {
	id := ttypes.WorkflowID{
		Namespace: "prod",
		Name:      "pipeline",
		Version:   3,
	}
	if id.Namespace != "prod" || id.Name != "pipeline" || id.Version != 3 {
		t.Errorf("unexpected WorkflowID: %+v", id)
	}
}

func TestDeliveryStatus_Values(t *testing.T) {
	statuses := []ttypes.DeliveryStatus{
		ttypes.DeliveryQueued, ttypes.DeliveryDelivered, ttypes.DeliveryFailed,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("empty delivery status")
		}
	}
}

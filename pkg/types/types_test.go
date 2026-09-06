package types

import (
	"testing"
	"time"
)

func TestRunState_IsTerminal(t *testing.T) {
	cases := []struct {
		state    RunState
		terminal bool
	}{
		{StatePending, false},
		{StateQueued, false},
		{StateRunning, false},
		{StateSucceeded, true},
		{StateFailed, true},
		{StateCancelled, true},
		{StateTimedOut, true},
	}
	for _, tc := range cases {
		if got := tc.state.IsTerminal(); got != tc.terminal {
			t.Errorf("RunState(%q).IsTerminal() = %v, want %v", tc.state, got, tc.terminal)
		}
	}
}

func TestParseRunState(t *testing.T) {
	s, err := ParseRunState("succeeded")
	if err != nil || s != StateSucceeded {
		t.Errorf("ParseRunState(succeeded) = %v, %v", s, err)
	}
	_, err = ParseRunState("bogus")
	if err == nil {
		t.Error("expected error for bogus state")
	}
}

func TestParseEventType(t *testing.T) {
	e, err := ParseEventType("run.created")
	if err != nil || e != EventRunCreated {
		t.Errorf("ParseEventType(run.created) = %v, %v", e, err)
	}
}

func TestEventType_Short(t *testing.T) {
	if s := EventRunCreated.Short(); s != "run-created" {
		t.Errorf("EventRunCreated.Short() = %q", s)
	}
}

func TestNewID(t *testing.T) {
	id1 := NewID()
	id2 := NewID()
	if id1 == "" || id2 == "" {
		t.Error("NewID returned empty")
	}
	if id1 == id2 {
		t.Error("NewID returned same value twice")
	}
}

func TestWorkflowDefinitionDefaults(t *testing.T) {
	now := time.Now()
	d := WorkflowDefinition{
		ID:        WorkflowID{Namespace: "ns", Name: "wf", Version: 1},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if d.ID.Name != "wf" {
		t.Errorf("expected name wf, got %s", d.ID.Name)
	}
}

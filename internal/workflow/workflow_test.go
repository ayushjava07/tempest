package workflow

import (
	"testing"
	"time"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestNewRun(t *testing.T) {
	now := time.Now()
	def := &ttypes.WorkflowDefinition{
		ID:    ttypes.WorkflowID{Namespace: "ns", Name: "wf", Version: 1},
		Steps: []ttypes.StepDefinition{{ID: "s1", Handler: "pass"}, {ID: "s2", Handler: "pass", DependsOn: []string{"s1"}}},
	}
	input := ttypes.RunInput{Workflow: ttypes.WorkflowID{Namespace: "ns", Name: "wf", Version: 1}}
	run, err := NewRun(def, input, now)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if run.State != ttypes.StatePending {
		t.Errorf("expected state PENDING, got %s", run.State)
	}
	if len(run.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(run.Steps))
	}
}

func TestNewRun_NilDef(t *testing.T) {
	_, err := NewRun(nil, ttypes.RunInput{}, time.Now())
	if err == nil {
		t.Error("expected error for nil definition")
	}
}

func TestNewRun_NoSteps(t *testing.T) {
	def := &ttypes.WorkflowDefinition{
		ID:    ttypes.WorkflowID{Name: "wf"},
		Steps: []ttypes.StepDefinition{},
	}
	_, err := NewRun(def, ttypes.RunInput{}, time.Now())
	if err == nil {
		t.Error("expected error for definition with no steps")
	}
}

func TestReadySteps(t *testing.T) {
	now := time.Now()
	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf"},
		Steps: []ttypes.StepDefinition{
			{ID: "a", Handler: "pass"},
			{ID: "b", Handler: "pass", DependsOn: []string{"a"}},
		},
	}
	run := &ttypes.Run{
		ID: "r1",
		Steps: []ttypes.StepRun{
			{StepID: "a", State: ttypes.StepPending},
			{StepID: "b", State: ttypes.StepPending},
		},
		CreatedAt: now,
	}
	ready := ReadySteps(def, run)
	if len(ready) != 1 || ready[0].StepID != "a" {
		t.Errorf("expected [a], got %v", ready)
	}
}

func TestReadySteps_AllDepsMet(t *testing.T) {
	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf"},
		Steps: []ttypes.StepDefinition{
			{ID: "a", Handler: "pass"},
			{ID: "b", Handler: "pass", DependsOn: []string{"a"}},
		},
	}
	run := &ttypes.Run{
		ID: "r1",
		Steps: []ttypes.StepRun{
			{StepID: "a", State: ttypes.StepSucceeded},
			{StepID: "b", State: ttypes.StepPending},
		},
	}
	ready := ReadySteps(def, run)
	if len(ready) != 1 || ready[0].StepID != "b" {
		t.Errorf("expected [b], got %v", ready)
	}
}

func TestReadySteps_NilInputs(t *testing.T) {
	if got := ReadySteps(nil, nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

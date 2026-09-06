package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/tempest-io/tempest/pkg/errors"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func NewRun(def *ttypes.WorkflowDefinition, input ttypes.RunInput, now time.Time) (*ttypes.Run, error) {
	if def == nil {
		return nil, errors.InvalidArgumentError(fmt.Errorf("definition must not be nil"))
	}
	if len(def.Steps) == 0 {
		return nil, errors.InvalidArgumentError(fmt.Errorf("definition must have at least one step"))
	}
	input.Workflow = def.ID
	steps := make([]ttypes.StepRun, len(def.Steps))
	for i, sd := range def.Steps {
		steps[i] = ttypes.StepRun{
			StepID: sd.ID, State: ttypes.StepPending,
			Output: make(map[string]any), Metadata: make(map[string]string),
		}
	}
	return &ttypes.Run{
		ID: fmt.Sprintf(":%x", time.Now().UnixNano()),
		Namespace: input.Workflow.Namespace,
		Workflow: input.Workflow,
		State: ttypes.StatePending,
		Input: input,
		Steps: steps,
		CreatedAt: now,
	}, nil
}

func ReadySteps(def *ttypes.WorkflowDefinition, run *ttypes.Run) []ttypes.StepRun {
	if def == nil || run == nil {
		return nil
	}
	deps := make(map[string][]string)
	for _, sd := range def.Steps {
		deps[sd.ID] = sd.DependsOn
	}
	stepState := make(map[string]ttypes.StepState, len(run.Steps))
	for _, s := range run.Steps {
		stepState[s.StepID] = s.State
	}
	var ready []ttypes.StepRun
	for _, s := range run.Steps {
		if s.State != ttypes.StepPending {
			continue
		}
		allDepsDone := true
		for _, dep := range deps[s.StepID] {
			if stepState[dep] != ttypes.StepSucceeded && stepState[dep] != ttypes.StepSkipped {
				allDepsDone = false
				break
			}
		}
		if allDepsDone {
			ready = append(ready, s)
		}
	}
	return ready
}

var _ = context.Background

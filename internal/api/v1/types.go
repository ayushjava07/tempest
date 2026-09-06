package v1

import (
	"time"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type WorkflowDefinition struct {
	Name      string `json:"name"`
	Version   int    `json:"version"`
	Namespace string `json:"namespace"`
	Steps     []Step `json:"steps,omitempty"`
}

type Step struct {
	ID       string   `json:"id"`
	Handler  string   `json:"handler"`
	Depends  []string `json:"depends,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`
	RetryMax int      `json:"retry_max,omitempty"`
}

type Run struct {
	ID        string      `json:"id"`
	Workflow  string      `json:"workflow"`
	Version   int         `json:"version"`
	Namespace string      `json:"namespace"`
	State     string      `json:"state"`
	Input     map[string]any `json:"input,omitempty"`
	Error     string      `json:"error,omitempty"`
	Steps     []StepState `json:"steps,omitempty"`
	CreatedAt string      `json:"created_at"`
}

type StepState struct {
	StepID  string         `json:"step_id"`
	State   string         `json:"state"`
	Attempt int            `json:"attempt"`
	Output  map[string]any `json:"output,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type SubmitRequest struct {
	Workflow string         `json:"workflow"`
	Version  int            `json:"version"`
	Input    map[string]any `json:"input,omitempty"`
}

type SubmitResponse struct {
	RunID string `json:"run_id"`
}

type ListResponse[T any] struct {
	Items []T  `json:"items"`
	Total int  `json:"total"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func ToRun(r *ttypes.Run) Run {
	out := Run{
		ID:        r.ID,
		Workflow:  r.Workflow.Name,
		Version:   r.Workflow.Version,
		Namespace: string(r.Namespace),
		State:     string(r.State),
		Input:     r.Input.Payload,
		Error:     r.Error,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
	out.Steps = make([]StepState, len(r.Steps))
	for i, s := range r.Steps {
		out.Steps[i] = StepState{
			StepID:  s.StepID,
			State:   string(s.State),
			Attempt: s.Attempt,
			Output:  s.Output,
			Error:   s.Error,
		}
	}
	return out
}

func ToDefinition(d *ttypes.WorkflowDefinition) WorkflowDefinition {
	out := WorkflowDefinition{
		Name:      d.ID.Name,
		Version:   d.ID.Version,
		Namespace: string(d.ID.Namespace),
	}
	out.Steps = make([]Step, len(d.Steps))
	for i, s := range d.Steps {
		out.Steps[i] = Step{
			ID:      s.ID,
			Handler: s.Handler,
			Depends: s.DependsOn,
			Timeout: s.Timeout.String(),
		}
	}
	return out
}

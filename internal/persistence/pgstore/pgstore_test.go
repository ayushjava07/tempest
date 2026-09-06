package pgstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/goleak"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStore_DataSerialization(t *testing.T) {
	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{
			Namespace: "default",
			Name:      "test-wf",
			Version:   1,
		},
		Steps: []ttypes.StepDefinition{
			{
				ID:      "step-1",
				Handler: "echo",
				Timeout: 10 * time.Second,
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	stepsJSON, err := json.Marshal(def.Steps)
	if err != nil {
		t.Fatalf("Marshal steps failed: %v", err)
	}

	var unmarshaledSteps []ttypes.StepDefinition
	if err := json.Unmarshal(stepsJSON, &unmarshaledSteps); err != nil {
		t.Fatalf("Unmarshal steps failed: %v", err)
	}

	if len(unmarshaledSteps) != 1 || unmarshaledSteps[0].ID != "step-1" {
		t.Errorf("unexpected unmarshaled steps: %v", unmarshaledSteps)
	}
}

func TestStore_RunStateSerialization(t *testing.T) {
	run := &ttypes.Run{
		ID:        "run-100",
		Namespace: "default",
		Workflow: ttypes.WorkflowID{
			Name:    "etl",
			Version: 2,
		},
		State: ttypes.StateRunning,
		Input: ttypes.RunInput{
			Payload: map[string]any{"dataset": "sales"},
		},
		Steps: []ttypes.StepRun{
			{
				StepID:  "step-extract",
				State:   ttypes.StepSucceeded,
				Attempt: 1,
			},
		},
		CreatedAt: time.Now().UTC(),
	}

	inputBytes, err := json.Marshal(run.Input)
	if err != nil {
		t.Fatalf("Marshal input failed: %v", err)
	}

	var decodedInput ttypes.RunInput
	if err := json.Unmarshal(inputBytes, &decodedInput); err != nil {
		t.Fatalf("Unmarshal input failed: %v", err)
	}

	if decodedInput.Payload["dataset"] != "sales" {
		t.Errorf("expected dataset=sales, got %v", decodedInput.Payload["dataset"])
	}
}

func TestStore_QueueItemFields(t *testing.T) {
	now := time.Now().UTC()
	item := ttypes.QueueItem{
		Namespace:  "default",
		RunID:      "run-q1",
		StepID:     "step-q1",
		EnqueuedAt: now,
		VisibleAt:  now.Add(30 * time.Second),
		Attempt:    1,
	}

	if item.RunID != "run-q1" || item.StepID != "step-q1" {
		t.Errorf("unexpected QueueItem fields: %+v", item)
	}
}

func TestStore_NilPoolSafety(t *testing.T) {
	// Verifies New constructor does not panic
	s := New(nil)
	if s == nil {
		t.Fatal("expected non-nil Store from New")
	}

	// Connect with invalid URL should fail gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Connect(ctx, "invalid-connection-string")
	if err == nil {
		t.Error("expected error from Connect with invalid URL")
	}
}

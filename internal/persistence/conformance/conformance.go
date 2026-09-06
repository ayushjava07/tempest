package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func RunAll(t *testing.T, store persistence.Store) {
	t.Run("Definitions", func(t *testing.T) {
		ctx := context.Background()
		def := &ttypes.WorkflowDefinition{
			ID: ttypes.WorkflowID{
				Namespace: "default",
				Name:      "test-wf",
				Version:   1,
			},
			Steps: []ttypes.StepDefinition{
				{ID: "step1", Handler: "pass"},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := store.CreateDefinition(ctx, def); err != nil {
			t.Fatalf("CreateDefinition: %v", err)
		}
		got, err := store.GetDefinitionByName(ctx, "default", "test-wf")
		if err != nil {
			t.Fatalf("GetDefinitionByName: %v", err)
		}
		if got.ID.Name != "test-wf" {
			t.Errorf("expected name test-wf, got %s", got.ID.Name)
		}
	})
	t.Run("Runs", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now()
		run := &ttypes.Run{
			ID:        "run-1",
			Namespace: "default",
			Workflow:  ttypes.WorkflowID{Namespace: "default", Name: "test-wf", Version: 1},
			State:     ttypes.StatePending,
			CreatedAt: now,
		}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
		got, err := store.GetRun(ctx, "default", "run-1")
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if got.ID != "run-1" {
			t.Errorf("expected run-1, got %s", got.ID)
		}
	})
	t.Run("Events", func(t *testing.T) {
		ctx := context.Background()
		ev := &ttypes.Event{
			ID:        "ev-1",
			Type:      ttypes.EventRunCreated,
			Version:   1,
			Namespace: "default",
			CreatedAt: time.Now(),
			Payload:   map[string]any{},
		}
		if err := store.AppendEvent(ctx, ev); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		events, err := store.ListEvents(ctx, persistence.EventFilter{Namespace: "default"})
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		if len(events) == 0 {
			t.Fatal("expected at least one event")
		}
	})
}

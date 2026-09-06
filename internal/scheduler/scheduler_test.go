package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/persistence/memstore"
	"github.com/tempest-io/tempest/internal/plugin"
	"github.com/tempest-io/tempest/pkg/types"
)

type nopHandler struct{}

func (h *nopHandler) Name() string        { return "pass" }
func (h *nopHandler) Description() string { return "no-op" }
func (h *nopHandler) Execute(_ context.Context, _ plugin.Handle) (plugin.Result, error) {
	return plugin.Result{Output: map[string]any{"ok": true}}, nil
}

func setupEngine(t *testing.T) (*Engine, *memstore.Store, func() time.Time) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var clockTime time.Time = now
	clock := func() time.Time { return clockTime }
	store := memstore.NewWithClock(clock)
	registry := plugin.NewRegistry()
	_ = registry.Register(&nopHandler{})
	def := &types.WorkflowDefinition{
		ID: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
		Steps: []types.StepDefinition{
			{ID: "s1", Handler: "pass"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = store.CreateDefinition(context.Background(), def)
	engine := NewEngine(store, registry, Options{Clock: clock})
	return engine, store, func() time.Time { return clockTime }
}

func TestSubmit(t *testing.T) {
	engine, _, _ := setupEngine(t)
	input := types.RunInput{
		Workflow: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
		Payload:  map[string]any{"key": "val"},
	}
	runID, err := engine.Submit(context.Background(), input)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if runID == "" {
		t.Error("expected non-empty run ID")
	}
}

func TestCancel(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var clockTime time.Time = now
	clock := func() time.Time { return clockTime }
	store := memstore.NewWithClock(clock)
	registry := plugin.NewRegistry()
	def := &types.WorkflowDefinition{
		ID: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
		Steps: []types.StepDefinition{
			{ID: "s1", Handler: "pass"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = store.CreateDefinition(context.Background(), def)
	engine := NewEngine(store, registry, Options{Clock: clock})
	input := types.RunInput{
		Workflow: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
	}
	runID, _ := engine.Submit(context.Background(), input)
	if err := engine.Cancel(context.Background(), "default", runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	run, _ := store.GetRun(context.Background(), "default", runID)
	if run.State != types.StateCancelled {
		t.Errorf("expected CANCELLED, got %s", run.State)
	}
}

func TestProcessLeases_ExecutesStep(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var clockTime time.Time = now
	clock := func() time.Time { return clockTime }
	store := memstore.NewWithClock(clock)
	registry := plugin.NewRegistry()
	_ = registry.Register(&nopHandler{})
	def := &types.WorkflowDefinition{
		ID: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
		Steps: []types.StepDefinition{
			{ID: "s1", Handler: "pass"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = store.CreateDefinition(context.Background(), def)
	engine := NewEngine(store, registry, Options{Clock: clock})
	input := types.RunInput{
		Workflow: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
	}
	runID, _ := engine.Submit(context.Background(), input)
	_ = store.UpdateRunState(context.Background(), "default", runID, types.StateRunning, now)
	// Advance clock so queue items become visible
	clockTime = now.Add(time.Second)
	_ = engine.ProcessLeases(context.Background())
	run, _ := store.GetRun(context.Background(), "default", runID)
	if run.State != types.StateSucceeded {
		t.Errorf("expected SUCCEEDED after ProcessLeases, got %s", run.State)
	}
}

func TestProcessLeases_MultipleSteps(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var clockTime time.Time = now
	clock := func() time.Time { return clockTime }
	store := memstore.NewWithClock(clock)
	registry := plugin.NewRegistry()
	_ = registry.Register(&nopHandler{})
	def := &types.WorkflowDefinition{
		ID: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
		Steps: []types.StepDefinition{
			{ID: "a", Handler: "pass"},
			{ID: "b", Handler: "pass", DependsOn: []string{"a"}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = store.CreateDefinition(context.Background(), def)
	engine := NewEngine(store, registry, Options{Clock: clock})
	input := types.RunInput{
		Workflow: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
	}
	runID, _ := engine.Submit(context.Background(), input)
	_ = store.UpdateRunState(context.Background(), "default", runID, types.StateRunning, now)
	// First pass: execute step 'a'
	clockTime = now.Add(time.Second)
	_ = engine.ProcessLeases(context.Background())
	// Second pass: execute step 'b' (now unblocked)
	clockTime = now.Add(2 * time.Second)
	_ = engine.ProcessLeases(context.Background())
	run, _ := store.GetRun(context.Background(), "default", runID)
	if run.State != types.StateSucceeded {
		t.Errorf("expected SUCCEEDED, got %s", run.State)
	}
}

func TestCancel_TerminalRun(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var clockTime time.Time = now
	clock := func() time.Time { return clockTime }
	store := memstore.NewWithClock(clock)
	registry := plugin.NewRegistry()
	_ = store.CreateDefinition(context.Background(), &types.WorkflowDefinition{
		ID: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1},
		Steps: []types.StepDefinition{{ID: "s1", Handler: "pass"}},
		CreatedAt: now, UpdatedAt: now,
	})
	engine := NewEngine(store, registry, Options{Clock: clock})
	input := types.RunInput{Workflow: types.WorkflowID{Namespace: "default", Name: "wf", Version: 1}}
	runID, _ := engine.Submit(context.Background(), input)
	_ = store.UpdateRunState(context.Background(), "default", runID, types.StateSucceeded, now)
	err := engine.Cancel(context.Background(), "default", runID)
	if err == nil {
		t.Error("expected error cancelling terminal run")
	}
}

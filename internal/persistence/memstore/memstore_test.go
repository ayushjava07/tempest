package memstore

import (
	"context"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestCreateAndGetDefinition(t *testing.T) {
	s := New()
	ctx := context.Background()
	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Namespace: "default", Name: "test", Version: 1},
		Steps: []ttypes.StepDefinition{{ID: "s1", Handler: "pass"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("CreateDefinition: %v", err)
	}
	got, err := s.GetDefinitionByName(ctx, "default", "test")
	if err != nil {
		t.Fatalf("GetDefinitionByName: %v", err)
	}
	if got.ID.Name != "test" {
		t.Errorf("expected test, got %s", got.ID.Name)
	}
}

func TestCreateDefinition_Duplicate(t *testing.T) {
	s := New()
	ctx := context.Background()
	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Namespace: "default", Name: "dup", Version: 1},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = s.CreateDefinition(ctx, def)
	if err := s.CreateDefinition(ctx, def); err == nil {
		t.Error("expected error for duplicate definition")
	}
}

func TestCreateAndGetRun(t *testing.T) {
	s := New()
	ctx := context.Background()
	now := time.Now()
	run := &ttypes.Run{
		ID: "r1", Namespace: "default",
		Workflow: ttypes.WorkflowID{Name: "wf", Version: 1},
		State: ttypes.StatePending, CreatedAt: now,
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	got, err := s.GetRun(ctx, "default", "r1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.ID != "r1" {
		t.Errorf("expected r1, got %s", got.ID)
	}
}

func TestListRuns(t *testing.T) {
	s := New()
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 3; i++ {
		_ = s.CreateRun(ctx, &ttypes.Run{
			ID: "r" + string(rune('1'+i)), Namespace: "default",
			Workflow: ttypes.WorkflowID{Name: "wf", Version: 1},
			State: ttypes.StatePending, CreatedAt: now,
		})
	}
	runs, err := s.ListRuns(ctx, persistence.RunFilter{Namespace: "default", Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runs))
	}
}

func TestUpdateRunState(t *testing.T) {
	s := New()
	ctx := context.Background()
	now := time.Now()
	_ = s.CreateRun(ctx, &ttypes.Run{
		ID: "r1", Namespace: "ns", CreatedAt: now,
		Workflow: ttypes.WorkflowID{Name: "wf", Version: 1},
		State: ttypes.StatePending,
	})
	if err := s.UpdateRunState(ctx, "ns", "r1", ttypes.StateRunning, now); err != nil {
		t.Fatalf("UpdateRunState: %v", err)
	}
	got, _ := s.GetRun(ctx, "ns", "r1")
	if got.State != ttypes.StateRunning {
		t.Errorf("expected RUNNING, got %s", got.State)
	}
	if got.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
}

func TestEnqueueAndDequeue(t *testing.T) {
	s := New()
	ctx := context.Background()
	now := time.Now()
	item := ttypes.QueueItem{
		RunID: "r1", Namespace: "default", StepID: "s1",
		EnqueuedAt: now, VisibleAt: now.Add(-time.Second), Attempt: 0,
	}
	if err := s.Enqueue(ctx, item); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	leases, err := s.Dequeue(ctx, "default", 10, now)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leases))
	}
}

func TestAppendAndGetEvent(t *testing.T) {
	s := New()
	ctx := context.Background()
	ev := &ttypes.Event{
		ID: "ev1", Type: ttypes.EventRunCreated, Version: 1,
		Namespace: "default", CreatedAt: time.Now(),
		Payload: map[string]any{"key": "val"},
	}
	if err := s.AppendEvent(ctx, ev); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	got, err := s.GetEvent(ctx, "ev1")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.ID != "ev1" {
		t.Errorf("expected ev1, got %s", got.ID)
	}
}

func TestCreateAndGetWebhookEndpoint(t *testing.T) {
	s := New()
	ctx := context.Background()
	ep := &ttypes.WebhookEndpoint{
		ID: "wh1", Namespace: "default", URL: "https://example.com/hook",
		Active: true, CreatedAt: time.Now(),
	}
	if err := s.CreateWebhookEndpoint(ctx, ep); err != nil {
		t.Fatalf("CreateWebhookEndpoint: %v", err)
	}
	got, err := s.GetWebhookEndpoint(ctx, "default", "wh1")
	if err != nil {
		t.Fatalf("GetWebhookEndpoint: %v", err)
	}
	if got.URL != "https://example.com/hook" {
		t.Errorf("expected URL, got %s", got.URL)
	}
}

func TestTokenCRUD(t *testing.T) {
	s := New()
	ctx := context.Background()
	tok := &ttypes.APIToken{
		ID: "t1", Namespace: "default", Role: "admin",
		TokenHash: "abc123", CreatedAt: time.Now(),
	}
	if err := s.CreateToken(ctx, tok); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	got, err := s.LookupTokenByHash(ctx, "abc123")
	if err != nil {
		t.Fatalf("LookupTokenByHash: %v", err)
	}
	if got.Role != "admin" {
		t.Errorf("expected admin, got %s", got.Role)
	}
}

func TestDeleteToken(t *testing.T) {
	s := New()
	ctx := context.Background()
	tok := &ttypes.APIToken{
		ID: "t1", Namespace: "default", Role: "reader",
		TokenHash: "hash1", CreatedAt: time.Now(),
	}
	_ = s.CreateToken(ctx, tok)
	if err := s.DeleteToken(ctx, "default", "t1"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	_, err := s.GetToken(ctx, "default", "t1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestClose(t *testing.T) {
	s := New()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

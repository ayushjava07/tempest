package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/auth"
	"github.com/tempest-io/tempest/internal/persistence/memstore"
	"github.com/tempest-io/tempest/internal/scheduler"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func setupTestGRPC(t *testing.T) (*Server, *memstore.Store) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := memstore.NewWithClock(func() time.Time { return now })
	resolver := auth.NewResolver(store)
	engine := scheduler.NewEngine(store, nil, scheduler.Options{
		Clock: func() time.Time { return now },
	})

	srv, err := NewGRPCServer(GRPCServerOptions{
		Store:    store,
		Engine:   engine,
		Resolver: resolver,
		Addr:     ":0",
	})
	if err != nil {
		t.Fatalf("failed to create grpc server: %v", err)
	}

	return srv, store
}

func TestGRPC_CreateDefinition(t *testing.T) {
	srv, _ := setupTestGRPC(t)
	ctx := context.Background()

	req := &CreateDefinitionRequest{
		Name:    "etl-pipeline",
		Version: 1,
		Steps: []*StepSpec{
			{Id: "extract", Handler: "pass"},
			{Id: "load", Handler: "pass", Depends: []string{"extract"}},
		},
	}

	resp, err := srv.CreateDefinition(ctx, req)
	if err != nil {
		t.Fatalf("failed to create definition: %v", err)
	}

	if resp.Name != "etl-pipeline" || resp.Version != 1 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestGRPC_SubmitAndGetRun(t *testing.T) {
	srv, store := setupTestGRPC(t)
	ctx := context.Background()

	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Namespace: "", Name: "data-sync", Version: 1},
		Steps: []ttypes.StepDefinition{
			{ID: "step1", Handler: "pass"},
		},
	}
	_ = store.CreateDefinition(ctx, def)

	subResp, err := srv.Submit(ctx, &SubmitRequest{
		Workflow: "data-sync",
		Version:  1,
	})
	if err != nil {
		t.Fatalf("failed to submit run: %v", err)
	}
	if subResp.RunId == "" {
		t.Fatalf("expected non-empty RunId")
	}

	runResp, err := srv.GetRun(ctx, &GetRunRequest{
		Namespace: "",
		RunId:     subResp.RunId,
	})
	if err != nil {
		t.Fatalf("failed to get run: %v", err)
	}
	if runResp.Id != subResp.RunId || runResp.Workflow != "data-sync" {
		t.Errorf("run response mismatch: %+v", runResp)
	}
}

func TestGRPC_CancelRun(t *testing.T) {
	srv, store := setupTestGRPC(t)
	ctx := context.Background()

	def := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Namespace: "", Name: "long-job", Version: 1},
		Steps: []ttypes.StepDefinition{
			{ID: "step1", Handler: "pass"},
		},
	}
	_ = store.CreateDefinition(ctx, def)

	subResp, err := srv.Submit(ctx, &SubmitRequest{
		Workflow: "long-job",
		Version:  1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cancelResp, err := srv.CancelRun(ctx, &CancelRunRequest{
		Namespace: "",
		RunId:     subResp.RunId,
	})
	if err != nil {
		t.Fatalf("failed to cancel run: %v", err)
	}
	if !cancelResp.Success {
		t.Errorf("expected success to be true")
	}
}

func TestGRPC_PublishEvent(t *testing.T) {
	srv, _ := setupTestGRPC(t)
	ctx := context.Background()

	resp, err := srv.PublishEvent(ctx, &PublishEventRequest{
		Type:      "run.custom_event",
		Namespace: "default",
		RunId:     "run-test",
		StepId:    "step-1",
		Payload:   map[string]any{"msg": "ok"},
	})
	if err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}
	if resp.EventId == "" {
		t.Errorf("expected non-empty EventId")
	}
}

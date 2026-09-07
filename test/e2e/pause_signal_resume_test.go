package e2e_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/approval"
)

type PipelineStatus string

const (
	StatusPending    PipelineStatus = "PENDING"
	StatusRunning    PipelineStatus = "RUNNING"
	StatusPaused     PipelineStatus = "PAUSED"
	StatusResumed    PipelineStatus = "RESUMED"
	StatusCompleted  PipelineStatus = "COMPLETED"
	StatusTerminated PipelineStatus = "TERMINATED"
)

type DeploymentPipeline struct {
	mu           sync.Mutex
	status       PipelineStatus
	stagesDone   []string
	resumeSignal chan struct{}
	abortSignal  chan struct{}
}

func NewDeploymentPipeline() *DeploymentPipeline {
	return &DeploymentPipeline{
		status:       StatusPending,
		stagesDone:   make([]string, 0),
		resumeSignal: make(chan struct{}, 1),
		abortSignal:  make(chan struct{}, 1),
	}
}

func (p *DeploymentPipeline) Status() PipelineStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

func (p *DeploymentPipeline) Execute(ctx context.Context, appMgr *approval.Manager, reqID string) error {
	p.mu.Lock()
	p.status = StatusRunning
	p.stagesDone = append(p.stagesDone, "build", "test", "deploy_staging")
	// Enter approval gate: pause pipeline
	p.status = StatusPaused
	p.mu.Unlock()

	// Wait for resume or abort signal or context cancellation
	select {
	case <-p.resumeSignal:
		p.mu.Lock()
		p.status = StatusResumed
		p.stagesDone = append(p.stagesDone, "deploy_production", "smoke_test")
		p.status = StatusCompleted
		p.mu.Unlock()
		return nil

	case <-p.abortSignal:
		p.mu.Lock()
		p.status = StatusTerminated
		p.mu.Unlock()
		return errors.New("deployment rejected or aborted")

	case <-ctx.Done():
		p.mu.Lock()
		p.status = StatusTerminated
		p.mu.Unlock()
		return ctx.Err()
	}
}

func TestE2E_WorkflowPauseApprovalGateAndResume(t *testing.T) {
	mgr := approval.NewManager()
	pipeline := NewDeploymentPipeline()

	approvers := []string{"lead-dev", "security-eng", "infra-lead"}
	req, err := mgr.CreateRequest(
		"deploy-prod-req-001",
		"production",
		"run-9988",
		"prod-gate",
		approval.PolicyQuorum,
		approvers,
		2, // Quorum: 2 out of 3 must approve
		10*time.Second,
		"vp-eng",
	)
	if err != nil {
		t.Fatalf("failed to create approval request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pipelineErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		pipelineErr = pipeline.Execute(ctx, mgr, req.ID)
	}()

	// Wait for pipeline to reach paused stage
	time.Sleep(50 * time.Millisecond)
	if status := pipeline.Status(); status != StatusPaused {
		t.Fatalf("expected pipeline to be PAUSED, got %v", status)
	}

	// 1. Unauthorized vote rejected
	_, err = mgr.CastVote(ctx, req.ID, "hacker", true, "approve")
	if !errors.Is(err, approval.ErrUnauthorizedPerson) {
		t.Fatalf("expected ErrUnauthorizedPerson, got %v", err)
	}

	// 2. First approver casts vote -> decision remains PENDING
	dec, err := mgr.CastVote(ctx, req.ID, "lead-dev", true, "lgtm release v2.0")
	if err != nil {
		t.Fatalf("unexpected vote error: %v", err)
	}
	if dec != approval.DecisionPending {
		t.Fatalf("expected DecisionPending after 1 vote, got %v", dec)
	}

	// Pipeline is still paused
	if status := pipeline.Status(); status != StatusPaused {
		t.Fatalf("expected pipeline to remain PAUSED, got %v", status)
	}

	// 3. Second approver casts vote -> reaches quorum (2/3) -> APPROVED
	dec, err = mgr.CastVote(ctx, req.ID, "security-eng", true, "security scan clean")
	if err != nil {
		t.Fatalf("unexpected vote error: %v", err)
	}
	if dec != approval.DecisionApproved {
		t.Fatalf("expected DecisionApproved after quorum reached, got %v", dec)
	}

	// 4. Send resume signal now that approval was granted
	pipeline.resumeSignal <- struct{}{}

	wg.Wait()

	if pipelineErr != nil {
		t.Fatalf("pipeline execution failed: %v", pipelineErr)
	}

	if status := pipeline.Status(); status != StatusCompleted {
		t.Fatalf("expected pipeline to reach COMPLETED, got %v", status)
	}

	pipeline.mu.Lock()
	expectedStages := []string{"build", "test", "deploy_staging", "deploy_production", "smoke_test"}
	if len(pipeline.stagesDone) != len(expectedStages) {
		t.Fatalf("unexpected stages: %v", pipeline.stagesDone)
	}
	for i, stage := range expectedStages {
		if pipeline.stagesDone[i] != stage {
			t.Errorf("stage [%d]: expected %s, got %s", i, stage, pipeline.stagesDone[i])
		}
	}
	pipeline.mu.Unlock()
}

func TestE2E_WorkflowPauseApprovalRejected(t *testing.T) {
	mgr := approval.NewManager()
	pipeline := NewDeploymentPipeline()

	approvers := []string{"lead-dev", "security-eng", "qa-lead"}
	req, err := mgr.CreateRequest(
		"deploy-reject-req-002",
		"production",
		"run-9989",
		"prod-gate",
		approval.PolicyAll, // Everyone must approve
		approvers,
		0,
		5*time.Second,
		"",
	)
	if err != nil {
		t.Fatalf("failed to create approval request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pipelineErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		pipelineErr = pipeline.Execute(ctx, mgr, req.ID)
	}()

	time.Sleep(50 * time.Millisecond)

	// QA lead rejects the deployment
	dec, err := mgr.CastVote(ctx, req.ID, "qa-lead", false, "critical blocker found in staging")
	if err != nil {
		t.Fatalf("unexpected vote error: %v", err)
	}
	if dec != approval.DecisionRejected {
		t.Fatalf("expected DecisionRejected, got %v", dec)
	}

	// Trigger pipeline abort signal
	pipeline.abortSignal <- struct{}{}

	wg.Wait()

	if pipelineErr == nil {
		t.Fatalf("expected pipeline to return rejection error")
	}

	if status := pipeline.Status(); status != StatusTerminated {
		t.Fatalf("expected pipeline to be TERMINATED, got %v", status)
	}
}

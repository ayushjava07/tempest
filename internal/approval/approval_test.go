package approval

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestApproval_PolicyAny(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	_, err := mgr.CreateRequest("req-1", "default", "run-1", "step-gate", PolicyAny, []string{"alice", "bob"}, 0, 1*time.Minute, "")
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	// Alice approves -> immediately approved
	dec, err := mgr.CastVote(ctx, "req-1", "alice", true, "LGTM")
	if err != nil {
		t.Fatalf("CastVote failed: %v", err)
	}
	if dec != DecisionApproved {
		t.Errorf("expected DecisionApproved, got %v", dec)
	}

	// Further votes rejected as already decided
	_, err = mgr.CastVote(ctx, "req-1", "bob", false, "too late")
	if !errors.Is(err, ErrAlreadyDecided) {
		t.Errorf("expected ErrAlreadyDecided, got %v", err)
	}
}

func TestApproval_PolicyAll(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	_, err := mgr.CreateRequest("req-2", "prod", "run-2", "step-deploy", PolicyAll, []string{"sec-lead", "ops-lead"}, 0, 1*time.Minute, "")
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	// First vote approves
	dec, _ := mgr.CastVote(ctx, "req-2", "sec-lead", true, "security pass")
	if dec != DecisionPending {
		t.Errorf("expected DecisionPending after 1/2 votes, got %v", dec)
	}

	// Second vote approves -> approved
	dec, _ = mgr.CastVote(ctx, "req-2", "ops-lead", true, "ops pass")
	if dec != DecisionApproved {
		t.Errorf("expected DecisionApproved after all approved, got %v", dec)
	}
}

func TestApproval_PolicyQuorum(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// 2 out of 3 quorum required
	_, _ = mgr.CreateRequest("req-3", "finance", "run-3", "step-payout", PolicyQuorum, []string{"mgr-1", "mgr-2", "mgr-3"}, 2, 1*time.Minute, "")

	// Vote 1: Reject
	dec, _ := mgr.CastVote(ctx, "req-3", "mgr-1", false, "not yet")
	if dec != DecisionPending {
		t.Errorf("expected DecisionPending (2 can still approve), got %v", dec)
	}

	// Vote 2: Approve
	dec, _ = mgr.CastVote(ctx, "req-3", "mgr-2", true, "agree")
	if dec != DecisionPending {
		t.Errorf("expected DecisionPending (1 approved, 1 needed), got %v", dec)
	}

	// Vote 3: Approve -> reaches quorum 2!
	dec, _ = mgr.CastVote(ctx, "req-3", "mgr-3", true, "agree too")
	if dec != DecisionApproved {
		t.Errorf("expected DecisionApproved on 2nd approval, got %v", dec)
	}
}

func TestApproval_UnauthorizedAndDuplicateVote(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	_, _ = mgr.CreateRequest("req-auth", "default", "run-x", "step-x", PolicyAny, []string{"charlie"}, 0, 1*time.Minute, "")

	// Stranger votes
	_, err := mgr.CastVote(ctx, "req-auth", "stranger", true, "fake")
	if !errors.Is(err, ErrUnauthorizedPerson) {
		t.Errorf("expected ErrUnauthorizedPerson, got %v", err)
	}

	// Charlie votes twice
	_, _ = mgr.CastVote(ctx, "req-auth", "charlie", false, "no")
	_, err = mgr.CastVote(ctx, "req-auth", "charlie", true, "retry")
	if !errors.Is(err, ErrAlreadyDecided) && !errors.Is(err, ErrDuplicateVote) {
		t.Errorf("expected error on duplicate vote, got %v", err)
	}
}

func TestApproval_Escalation(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	now := time.Now().UTC()
	_, _ = mgr.CreateRequest("req-esc", "default", "run-esc", "step-esc", PolicyAny, []string{"lead"}, 0, 50*time.Millisecond, "vp_eng")

	// Not escalated initially
	if mgr.CheckEscalation("req-esc", now) {
		t.Error("should not escalate before timeout")
	}

	// After timeout
	future := now.Add(100 * time.Millisecond)
	if !mgr.CheckEscalation("req-esc", future) {
		t.Error("expected escalation after timeout")
	}

	// Now vp_eng can approve!
	dec, err := mgr.CastVote(ctx, "req-esc", "vp_eng", true, "executive override")
	if err != nil || dec != DecisionApproved {
		t.Errorf("expected vp_eng to be able to approve after escalation: %v (%v)", dec, err)
	}
}

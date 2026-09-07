package leader

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestLeader_DualCandidateContest(t *testing.T) {
	coord := NewInMemCoordinator()
	cfg := Config{
		LeaseDuration: 100 * time.Millisecond,
		RenewInterval: 30 * time.Millisecond,
		RetryInterval: 20 * time.Millisecond,
	}

	c1 := NewCandidate("node-1", coord, cfg)
	c2 := NewCandidate("node-2", coord, cfg)

	won1, err := c1.Campaign()
	if err != nil || !won1 {
		t.Fatalf("expected node-1 to win leadership: %v", err)
	}

	// Node 2 must fail to win while Node 1 holds lease
	won2, err := c2.Campaign()
	if err != nil || won2 {
		t.Fatalf("node-2 should not win while node-1 holds lease")
	}

	if c1.State() != StateLeader {
		t.Fatalf("node-1 should be StateLeader, got %s", c1.State())
	}
	if c2.State() != StateFollower {
		t.Fatalf("node-2 should be StateFollower, got %s", c2.State())
	}

	// Term must be > 0
	if c1.Term() == 0 {
		t.Fatalf("expected positive fencing term, got %d", c1.Term())
	}
}

func TestLeader_HeartbeatRenewal(t *testing.T) {
	coord := NewInMemCoordinator()
	cfg := Config{
		LeaseDuration: 80 * time.Millisecond,
		RenewInterval: 20 * time.Millisecond,
		RetryInterval: 20 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c1 := NewCandidate("active-leader", coord, cfg)
	c1.Start(ctx)

	// Wait for leadership acquisition
	time.Sleep(50 * time.Millisecond)

	if c1.State() != StateLeader {
		t.Fatalf("expected active-leader to acquire leadership")
	}

	// Wait across several lease expiration periods (150ms > 80ms)
	time.Sleep(150 * time.Millisecond)

	// Still leader because heartbeats renewed the lease
	if c1.State() != StateLeader {
		t.Fatalf("expected active-leader to retain leadership through renewal")
	}

	c1.Stop()
}

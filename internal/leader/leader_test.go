package leader

import (
	"context"
	"sync/atomic"
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

func TestLeader_StepDownAndSuccessorTakeover(t *testing.T) {
	coord := NewInMemCoordinator()
	cfg := Config{
		LeaseDuration: 50 * time.Millisecond,
		RenewInterval: 15 * time.Millisecond,
		RetryInterval: 15 * time.Millisecond,
	}

	ctx := context.Background()

	c1 := NewCandidate("node-primary", coord, cfg)
	c2 := NewCandidate("node-standby", coord, cfg)

	c1.Start(ctx)
	time.Sleep(30 * time.Millisecond)

	if c1.State() != StateLeader {
		t.Fatalf("primary should be leader")
	}

	c2.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	if c2.State() != StateFollower {
		t.Fatalf("standby should be follower while primary is healthy")
	}

	// Primary steps down
	c1.Stop()
	time.Sleep(50 * time.Millisecond)

	// Standby should detect vacancy and take over
	if c2.State() != StateLeader {
		t.Fatalf("standby should take over leadership after primary steps down")
	}

	// Term must advance
	if c2.Term() <= c1.Term() {
		t.Fatalf("expected successor term (%d) to be higher than previous (%d)", c2.Term(), c1.Term())
	}

	c2.Stop()
}

func TestLeader_ContextCancellation(t *testing.T) {
	coord := NewInMemCoordinator()
	cfg := Config{
		LeaseDuration: 50 * time.Millisecond,
		RenewInterval: 15 * time.Millisecond,
		RetryInterval: 15 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := NewCandidate("ctx-node", coord, cfg)
	c.Start(ctx)
	time.Sleep(25 * time.Millisecond)

	if c.State() != StateLeader {
		t.Fatalf("expected ctx-node to become leader")
	}

	// Cancel context -> should step down
	cancel()
	time.Sleep(30 * time.Millisecond)

	if c.State() != StateFollower {
		t.Fatalf("expected ctx-node to step down to follower after context cancel, got %s", c.State())
	}
}

func TestLeader_ObserverHooks(t *testing.T) {
	coord := NewInMemCoordinator()
	cfg := Config{
		LeaseDuration: 50 * time.Millisecond,
		RenewInterval: 15 * time.Millisecond,
		RetryInterval: 15 * time.Millisecond,
	}

	c := NewCandidate("obs-node", coord, cfg)

	var electedFired, revokedFired atomic.Bool
	c.OnElected(func() {
		electedFired.Store(true)
	})
	c.OnRevoked(func() {
		revokedFired.Store(true)
	})

	ctx := context.Background()
	c.Start(ctx)
	time.Sleep(30 * time.Millisecond)

	if !electedFired.Load() {
		t.Fatalf("expected OnElected callback to trigger")
	}

	c.Stop()
	time.Sleep(20 * time.Millisecond)

	if !revokedFired.Load() {
		t.Fatalf("expected OnRevoked callback to trigger on Stop")
	}
}

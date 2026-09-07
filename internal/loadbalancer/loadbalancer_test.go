package loadbalancer

import (
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBalancer_RoundRobin(t *testing.T) {
	b := New(StrategyRoundRobin)
	_ = b.AddTarget("node-1", "10.0.0.1:8080", 1)
	_ = b.AddTarget("node-2", "10.0.0.2:8080", 1)

	counts := make(map[string]int)
	for i := 0; i < 10; i++ {
		target, done, err := b.Select("")
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[target.ID]++
		done()
	}

	if counts["node-1"] != 5 || counts["node-2"] != 5 {
		t.Fatalf("expected exact 5/5 split, got: %v", counts)
	}
}

func TestBalancer_WeightedRoundRobin(t *testing.T) {
	b := New(StrategyWeightedRoundRobin)
	_ = b.AddTarget("heavy", "10.0.0.1", 3)
	_ = b.AddTarget("light", "10.0.0.2", 1)

	counts := make(map[string]int)
	for i := 0; i < 40; i++ {
		target, done, err := b.Select("")
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[target.ID]++
		done()
	}

	if counts["heavy"] != 30 || counts["light"] != 10 {
		t.Fatalf("expected 30/10 distribution for 3:1 weight ratio, got: %v", counts)
	}
}

func TestBalancer_LeastConnections(t *testing.T) {
	b := New(StrategyLeastConnections)
	_ = b.AddTarget("srv-1", "10.0.0.1", 1)
	_ = b.AddTarget("srv-2", "10.0.0.2", 1)

	// First request goes to srv-1 (alphabetically first)
	t1, done1, _ := b.Select("")
	if t1.ID != "srv-1" {
		t.Fatalf("expected srv-1, got %s", t1.ID)
	}

	// While srv-1 is holding a connection, second request MUST go to srv-2
	t2, done2, _ := b.Select("")
	if t2.ID != "srv-2" {
		t.Fatalf("expected srv-2 for least connections, got %s", t2.ID)
	}

	done1()
	done2()
}

func TestBalancer_ConsistentHash(t *testing.T) {
	b := New(StrategyConsistentHash)
	_ = b.AddTarget("shard-A", "10.0.0.1", 1)
	_ = b.AddTarget("shard-B", "10.0.0.2", 1)
	_ = b.AddTarget("shard-C", "10.0.0.3", 1)

	key1 := "user-order-12345"
	key2 := "invoice-999"

	// Selecting with same key repeatedly must land on the exact same target
	target1A, done, _ := b.Select(key1)
	done()
	target1B, done, _ := b.Select(key1)
	done()

	if target1A.ID != target1B.ID {
		t.Fatalf("consistent hash mismatch for key1: %s vs %s", target1A.ID, target1B.ID)
	}

	target2, done, _ := b.Select(key2)
	done()
	if target2 == nil {
		t.Fatalf("expected target for key2")
	}

	// Mark chosen shard unhealthy -> must failover consistently
	_ = b.SetHealth(target1A.ID, false)
	failoverTarget, done, err := b.Select(key1)
	if err != nil {
		t.Fatalf("failover select failed: %v", err)
	}
	done()
	if failoverTarget.ID == target1A.ID {
		t.Fatalf("expected failover target to differ from unhealthy node")
	}
}

func TestBalancer_NoHealthyTargets(t *testing.T) {
	b := New(StrategyRoundRobin)
	_ = b.AddTarget("single", "10.0.0.1", 1)
	_ = b.SetHealth("single", false)

	_, _, err := b.Select("")
	if err != ErrNoHealthyTargets {
		t.Fatalf("expected ErrNoHealthyTargets, got %v", err)
	}
}

func TestBalancer_Concurrency(t *testing.T) {
	b := New(StrategyPowerOfTwoChoices)
	for i := 0; i < 5; i++ {
		_ = b.AddTarget(fmt.Sprintf("worker-%d", i), "localhost", 1)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				tgt, done, err := b.Select("")
				if err != nil {
					t.Errorf("concurrent select error: %v", err)
					return
				}
				if tgt.ActiveConns.Load() < 1 {
					t.Errorf("active conns should be >= 1 while in-flight")
				}
				done()
			}
		}()
	}

	wg.Wait()
}

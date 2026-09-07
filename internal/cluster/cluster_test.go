package cluster

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCluster_RegisterAndDeregister(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	err := coord.Register("node-1", "10.0.0.1:8080", "worker")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Duplicate register must fail
	err = coord.Register("node-1", "10.0.0.1:8080", "worker")
	if !errors.Is(err, ErrNodeAlreadyAlive) {
		t.Errorf("expected ErrNodeAlreadyAlive, got %v", err)
	}

	active := coord.ActiveNodes()
	if len(active) != 1 || active[0] != "node-1" {
		t.Errorf("unexpected active nodes: %v", active)
	}

	// Deregister
	err = coord.Deregister("node-1")
	if err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}
	if len(coord.ActiveNodes()) != 0 {
		t.Errorf("expected 0 active nodes after deregister")
	}

	// Deregister nonexistent
	err = coord.Deregister("node-unknown")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestCluster_PhiAccrualFailureDetection(t *testing.T) {
	coord := NewCoordinator(Config{
		SuspectThreshold: 4.0,
		DeadThreshold:    8.0,
		HeartbeatHistory: 20,
	})

	_ = coord.Register("node-monitored", "10.0.0.2:8080", "worker")

	base := time.Now()
	// Train failure detector with 10 steady heartbeats spaced 1.0s apart
	for i := 1; i <= 10; i++ {
		tStep := base.Add(time.Duration(i) * time.Second)
		coord.nodes["node-monitored"].recordHeartbeat(tStep)
	}

	// Immediate check: should be ALIVE with Phi ~ 0
	lastHb := base.Add(10 * time.Second)
	states := coord.EvaluateHealth(lastHb.Add(500 * time.Millisecond))
	if states["node-monitored"] != StateAlive {
		t.Errorf("expected StateAlive right after heartbeats, got %v", states["node-monitored"])
	}

	// Advance time by 2.0s without heartbeat: Phi should exceed SuspectThreshold
	suspectTime := lastHb.Add(2 * time.Second)
	states = coord.EvaluateHealth(suspectTime)
	if states["node-monitored"] != StateSuspect {
		t.Errorf("expected StateSuspect after 2s silence, got %v (phi=%f)",
			states["node-monitored"], coord.nodes["node-monitored"].Phi(suspectTime))
	}

	// Advance time by 4.0s: Phi should exceed DeadThreshold
	deadTime := lastHb.Add(4 * time.Second)
	states = coord.EvaluateHealth(deadTime)
	if states["node-monitored"] != StateDead {
		t.Errorf("expected StateDead after 4s silence, got %v (phi=%f)",
			states["node-monitored"], coord.nodes["node-monitored"].Phi(deadTime))
	}

	// Fresh heartbeat revives node back to ALIVE!
	_ = coord.Heartbeat("node-monitored")
	states = coord.EvaluateHealth(time.Now())
	if states["node-monitored"] != StateAlive {
		t.Errorf("expected StateAlive after fresh heartbeat, got %v", states["node-monitored"])
	}
}

func TestCluster_Concurrency(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())
	numNodes := 10

	for i := 0; i < numNodes; i++ {
		_ = coord.Register(fmt.Sprintf("node-%d", i), fmt.Sprintf("10.0.0.%d", i), "worker")
	}

	var wg sync.WaitGroup
	// Concurrently send heartbeats and evaluate health
	for i := 0; i < numNodes; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("node-%d", id)
			for j := 0; j < 20; j++ {
				_ = coord.Heartbeat(nodeID)
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 20; j++ {
			_ = coord.EvaluateHealth(time.Now())
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}

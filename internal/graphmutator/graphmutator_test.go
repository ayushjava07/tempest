package graphmutator

import (
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDAG_BasicAndTopologicalSort(t *testing.T) {
	dag := New()

	_ = dag.AddNode(&Node{ID: "compile", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "test", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "package", Type: NodeTask})

	_ = dag.AddEdge("compile", "test")
	_ = dag.AddEdge("test", "package")

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected sort error: %v", err)
	}

	expected := []string{"compile", "test", "package"}
	if len(order) != len(expected) {
		t.Fatalf("expected order len %d, got %d", len(expected), len(order))
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("order mismatch at %d: expected %s, got %s", i, expected[i], order[i])
		}
	}
}

func TestDAG_CycleDetection(t *testing.T) {
	dag := New()

	_ = dag.AddNode(&Node{ID: "A", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "B", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "C", Type: NodeTask})

	_ = dag.AddEdge("A", "B")
	_ = dag.AddEdge("B", "C")
	_ = dag.AddEdge("C", "A") // introduces cycle

	_, err := dag.TopologicalSort()
	if err == nil || err != ErrCycleDetected {
		t.Fatalf("expected ErrCycleDetected, got: %v", err)
	}
}

func TestDAG_InjectBefore(t *testing.T) {
	dag := New()

	_ = dag.AddNode(&Node{ID: "build", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "deploy", Type: NodeTask})
	_ = dag.AddEdge("build", "deploy")

	// Inject approval before deploy
	approvalNode := &Node{ID: "approve_deploy", Type: NodeApproval}
	err := dag.InjectBefore("deploy", approvalNode)
	if err != nil {
		t.Fatalf("InjectBefore failed: %v", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("sort failed: %v", err)
	}

	expected := []string{"build", "approve_deploy", "deploy"}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("order mismatch at %d: want %s, got %s", i, expected[i], order[i])
		}
	}
}

func TestDAG_InjectAfter(t *testing.T) {
	dag := New()

	_ = dag.AddNode(&Node{ID: "deploy", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "notify", Type: NodeTask})
	_ = dag.AddEdge("deploy", "notify")

	// Inject verify step after deploy
	verifyNode := &Node{ID: "verify_health", Type: NodeTask}
	err := dag.InjectAfter("deploy", verifyNode)
	if err != nil {
		t.Fatalf("InjectAfter failed: %v", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("sort failed: %v", err)
	}

	expected := []string{"deploy", "verify_health", "notify"}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("order mismatch at %d: want %s, got %s", i, expected[i], order[i])
		}
	}
}

func TestDAG_ExpandFanOut(t *testing.T) {
	dag := New()

	_ = dag.AddNode(&Node{ID: "fetch_partitions", Type: NodeTask})
	_ = dag.AddNode(&Node{ID: "process_map", Type: NodeFanOut})
	_ = dag.AddNode(&Node{ID: "publish", Type: NodeTask})

	_ = dag.AddEdge("fetch_partitions", "process_map")
	_ = dag.AddEdge("process_map", "publish")

	// Dynamically expand process_map into 3 parallel workers + 1 reduce node
	err := dag.ExpandFanOut(
		"process_map",
		3,
		func(i int) *Node {
			return &Node{
				ID:   fmt.Sprintf("worker_%d", i),
				Type: NodeTask,
			}
		},
		&Node{
			ID:   "reduce_summary",
			Type: NodeFanIn,
		},
	)
	if err != nil {
		t.Fatalf("ExpandFanOut failed: %v", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed after fan-out: %v", err)
	}

	if len(order) != 6 { // fetch_partitions + 3 workers + reduce_summary + publish
		t.Fatalf("expected 6 nodes in sorted order, got %d: %v", len(order), order)
	}

	// publish must depend on reduce_summary
	pubNode, _ := dag.GetNode("publish")
	if len(pubNode.Dependencies) != 1 || pubNode.Dependencies[0] != "reduce_summary" {
		t.Fatalf("publish should depend on reduce_summary, got: %v", pubNode.Dependencies)
	}
}

func TestDAG_InlineSubDAG(t *testing.T) {
	mainDAG := New()
	_ = mainDAG.AddNode(&Node{ID: "init", Type: NodeTask})
	_ = mainDAG.AddNode(&Node{ID: "subworkflow_task", Type: NodeSubDAG})
	_ = mainDAG.AddNode(&Node{ID: "cleanup", Type: NodeTask})

	_ = mainDAG.AddEdge("init", "subworkflow_task")
	_ = mainDAG.AddEdge("subworkflow_task", "cleanup")

	// Build sub-DAG: sub1 -> sub2
	subDAG := New()
	_ = subDAG.AddNode(&Node{ID: "sub1", Type: NodeTask})
	_ = subDAG.AddNode(&Node{ID: "sub2", Type: NodeTask})
	_ = subDAG.AddEdge("sub1", "sub2")

	err := mainDAG.InlineSubDAG("subworkflow_task", subDAG)
	if err != nil {
		t.Fatalf("InlineSubDAG failed: %v", err)
	}

	order, err := mainDAG.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed after inlining: %v", err)
	}

	expected := []string{"init", "sub1", "sub2", "cleanup"}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("order mismatch at %d: expected %s, got %s", i, expected[i], order[i])
		}
	}
}

func TestDAG_DiffAndClone(t *testing.T) {
	d1 := New()
	_ = d1.AddNode(&Node{ID: "step1", Type: NodeTask})
	_ = d1.AddNode(&Node{ID: "step2", Type: NodeTask})

	d2 := d1.Clone()
	_ = d2.AddNode(&Node{ID: "step3", Type: NodeTask})
	_ = d2.RemoveNode("step1")

	diff := d1.Diff(d2)
	if len(diff.AddedNodes) != 1 || diff.AddedNodes[0] != "step3" {
		t.Fatalf("expected step3 added, got %v", diff.AddedNodes)
	}
	if len(diff.RemovedNodes) != 1 || diff.RemovedNodes[0] != "step1" {
		t.Fatalf("expected step1 removed, got %v", diff.RemovedNodes)
	}
}

func TestDAG_Concurrency(t *testing.T) {
	dag := New()
	_ = dag.AddNode(&Node{ID: "root", Type: NodeTask})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("node_%d", idx)
			_ = dag.AddNode(&Node{ID: nodeID, Type: NodeTask})
			_ = dag.AddEdge("root", nodeID)
			_, _ = dag.GetNode(nodeID)
		}(i)
	}
	wg.Wait()

	if err := dag.Validate(); err != nil {
		t.Fatalf("dag validation failed after concurrent additions: %v", err)
	}
}

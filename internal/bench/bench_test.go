package bench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/dag"
	"github.com/tempest-io/tempest/internal/expression"
	"github.com/tempest-io/tempest/internal/persistence/memstore"
	"github.com/tempest-io/tempest/internal/statemachine"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func BenchmarkStateMachine_LegalTransition(b *testing.B) {
	sm := statemachine.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.CanTransition(ttypes.StatePending, ttypes.StateQueued)
	}
}

func BenchmarkDAG_TopologicalSort_10Nodes(b *testing.B) {
	g := dag.New()
	for i := 0; i < 10; i++ {
		g.AddNode(fmt.Sprintf("n%d", i))
	}
	for i := 0; i < 9; i++ {
		_ = g.AddEdge(fmt.Sprintf("n%d", i), fmt.Sprintf("n%d", i+1))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = g.TopologicalSort()
	}
}

func BenchmarkExpression_EvaluateVariable(b *testing.B) {
	eval := expression.New()
	ctx := map[string]interface{}{
		"step": map[string]interface{}{
			"status": "success",
			"code":   200,
		},
	}
	expr := "${step.code} == 200"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate(expr, ctx)
	}
}

func BenchmarkMemStore_CreateAndGetRun(b *testing.B) {
	store := memstore.New()
	ctx := context.Background()
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("run-%d", i)
		r := &ttypes.Run{
			ID:        id,
			Namespace: "default",
			Workflow:  ttypes.WorkflowID{Namespace: "default", Name: "test", Version: 1},
			State:     ttypes.StatePending,
			CreatedAt: now,
		}
		_ = store.CreateRun(ctx, r)
		_, _ = store.GetRun(ctx, "default", id)
	}
}

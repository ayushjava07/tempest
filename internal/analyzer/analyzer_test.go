package analyzer

import (
	"testing"
)

func TestUnreachableAndMissingDependency(t *testing.T) {
	ast := &WorkflowAST{
		WorkflowID: "wf-reach-test",
		Steps: map[string]StepNode{
			"root": {
				ID: "root",
			},
			"step-child": {
				ID:        "step-child",
				DependsOn: []string{"root"},
			},
			"orphan": {
				ID:        "orphan",
				DependsOn: []string{"non-existent-step"},
			},
		},
	}

	reachRule := UnreachableStepRule{}
	reachDiags := reachRule.Analyze(ast)
	if len(reachDiags) != 1 {
		t.Fatalf("expected 1 unreachable diagnostic, got %d", len(reachDiags))
	}
	if reachDiags[0].StepID != "orphan" {
		t.Fatalf("expected orphan step flagged, got %s", reachDiags[0].StepID)
	}

	missingRule := MissingDependencyRule{}
	missDiags := missingRule.Analyze(ast)
	if len(missDiags) != 1 {
		t.Fatalf("expected 1 missing dependency diagnostic, got %d", len(missDiags))
	}
	if missDiags[0].StepID != "orphan" {
		t.Fatalf("expected orphan flagged for missing dep, got %s", missDiags[0].StepID)
	}
}

func TestCyclicAndSelfDependency(t *testing.T) {
	// 1. Test 3-node cycle: A -> B -> C -> A
	cycleAST := &WorkflowAST{
		WorkflowID: "wf-cycle",
		Steps: map[string]StepNode{
			"A": {ID: "A", DependsOn: []string{"B"}},
			"B": {ID: "B", DependsOn: []string{"C"}},
			"C": {ID: "C", DependsOn: []string{"A"}},
		},
	}

	cycleRule := CyclicDependencyRule{}
	diags := cycleRule.Analyze(cycleAST)
	if len(diags) == 0 {
		t.Fatalf("expected cyclic dependency diagnostic detected")
	}

	// 2. Test self-referential step: X -> X
	selfAST := &WorkflowAST{
		WorkflowID: "wf-self",
		Steps: map[string]StepNode{
			"X": {ID: "X", DependsOn: []string{"X"}},
		},
	}
	selfDiags := cycleRule.Analyze(selfAST)
	if len(selfDiags) != 1 {
		t.Fatalf("expected 1 self-referential diagnostic, got %d", len(selfDiags))
	}
}

func TestMissingParameterAndResourceQuota(t *testing.T) {
	ast := &WorkflowAST{
		WorkflowID: "wf-params",
		Parameters: map[string]string{
			"declared_var": "hello",
		},
		Steps: map[string]StepNode{
			"step-ok": {
				ID:      "step-ok",
				Command: "echo ${inputs.declared_var}",
				Timeout: "30s",
				Retries: 3,
			},
			"step-bad": {
				ID:      "step-bad",
				Command: "echo ${inputs.missing_var} and ${steps.ghost_step.out}",
				Timeout: "invalid-time",
				Retries: -1,
			},
		},
	}

	paramRule := MissingParameterRule{}
	paramDiags := paramRule.Analyze(ast)
	if len(paramDiags) != 2 {
		t.Fatalf("expected 2 parameter diagnostics, got %d", len(paramDiags))
	}

	quotaRule := NewDefaultResourceQuotaRule()
	quotaDiags := quotaRule.Analyze(ast)
	if len(quotaDiags) < 2 {
		t.Fatalf("expected invalid timeout and negative retry diagnostics, got %d", len(quotaDiags))
	}

	// Integration test with Analyzer
	a := New()
	a.Register(UnreachableStepRule{})
	a.Register(MissingDependencyRule{})
	a.Register(CyclicDependencyRule{})
	a.Register(paramRule)
	a.Register(quotaRule)

	allDiags := a.Analyze(ast)
	if !HasErrors(allDiags) {
		t.Fatalf("expected HasErrors=true due to missing parameters and invalid timeouts")
	}
}

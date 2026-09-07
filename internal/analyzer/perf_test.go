package analyzer

import (
	"fmt"
	"testing"
)

func TestConfigSuppressionAndOverride(t *testing.T) {
	configJSON := []byte(`{
		"disabled_rules": ["WF-001"],
		"step_suppressions": {
			"step-legacy": ["SEC-001", "WF-006"]
		},
		"severity_overrides": {
			"WF-004": "WARNING"
		}
	}`)

	cfg, err := ParseConfig(configJSON)
	if err != nil {
		t.Fatalf("unexpected ParseConfig error: %v", err)
	}

	// WF-001 is globally suppressed
	if !cfg.IsSuppressed("WF-001", "any-step") {
		t.Fatalf("expected WF-001 to be globally suppressed")
	}

	// SEC-001 is suppressed for step-legacy
	if !cfg.IsSuppressed("SEC-001", "step-legacy") {
		t.Fatalf("expected SEC-001 suppressed on step-legacy")
	}

	// SEC-001 is NOT suppressed on other-step
	if cfg.IsSuppressed("SEC-001", "other-step") {
		t.Fatalf("SEC-001 should not be suppressed on other-step")
	}

	// Severity override
	effSev := cfg.EffectiveSeverity("WF-004", SeverityError)
	if effSev != SeverityWarning {
		t.Fatalf("expected overridden severity WARNING, got %s", effSev)
	}

	rawDiags := []Diagnostic{
		{RuleID: "WF-001", StepID: "step-1", Severity: SeverityWarning},
		{RuleID: "SEC-001", StepID: "step-legacy", Severity: SeverityError},
		{RuleID: "SEC-001", StepID: "other-step", Severity: SeverityError},
		{RuleID: "WF-004", StepID: "cycle-step", Severity: SeverityError},
	}

	filtered := cfg.FilterDiagnostics(rawDiags)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered diagnostics, got %d", len(filtered))
	}
	if filtered[0].RuleID != "SEC-001" || filtered[0].StepID != "other-step" {
		t.Fatalf("unexpected first diag: %+v", filtered[0])
	}
	if filtered[1].RuleID != "WF-004" || filtered[1].Severity != SeverityWarning {
		t.Fatalf("unexpected second diag with overridden severity: %+v", filtered[1])
	}
}

func TestLargeGraphAnalyzerPerformance(t *testing.T) {
	// Construct a synthetic DAG with 500 connected steps
	ast := &WorkflowAST{
		WorkflowID: "wf-bench-500",
		Steps:      make(map[string]StepNode),
		Parameters: map[string]string{"env": "prod"},
	}

	// Root node
	ast.Steps["step-0"] = StepNode{
		ID:      "step-0",
		Command: "echo ${inputs.env}",
		Timeout: "10s",
		Retries: 2,
	}

	for i := 1; i < 500; i++ {
		parent := fmt.Sprintf("step-%d", i-1)
		curr := fmt.Sprintf("step-%d", i)
		ast.Steps[curr] = StepNode{
			ID:        curr,
			DependsOn: []string{parent},
			Command:   fmt.Sprintf("echo processing %s", curr),
			Timeout:   "10s",
			Retries:   2,
		}
	}

	a := New()
	a.Register(UnreachableStepRule{})
	a.Register(MissingDependencyRule{})
	a.Register(CyclicDependencyRule{})
	a.Register(UnboundedLoopRule{})
	a.Register(MissingParameterRule{})
	a.Register(NewDefaultResourceQuotaRule())
	a.Register(ShellInjectionRule{})

	diags := a.Analyze(ast)
	if len(diags) != 0 {
		t.Fatalf("expected clean 500-node linear DAG to have 0 diagnostics, got %d: %v", len(diags), diags)
	}
}

func BenchmarkLargeDAGAnalysis(b *testing.B) {
	ast := &WorkflowAST{
		WorkflowID: "wf-bench-bench",
		Steps:      make(map[string]StepNode),
		Parameters: map[string]string{"env": "prod"},
	}

	ast.Steps["step-0"] = StepNode{ID: "step-0"}
	for i := 1; i < 200; i++ {
		parent := fmt.Sprintf("step-%d", i-1)
		curr := fmt.Sprintf("step-%d", i)
		ast.Steps[curr] = StepNode{
			ID:        curr,
			DependsOn: []string{parent},
		}
	}

	a := New()
	a.Register(UnreachableStepRule{})
	a.Register(MissingDependencyRule{})
	a.Register(CyclicDependencyRule{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Analyze(ast)
	}
}

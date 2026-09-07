package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICommandRoutingAndExitCodes(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// 1. version command
	err := runWith([]string{"version"}, stdout, stderr)
	if err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "tempest v") {
		t.Fatalf("unexpected version output: %s", stdout.String())
	}

	// 2. unknown command returns error
	stdout.Reset()
	stderr.Reset()
	err = runWith([]string{"nonexistent-command"}, stdout, stderr)
	if err == nil {
		t.Fatalf("expected error for unknown command")
	}

	// 3. workflow submit without args returns usage error
	stdout.Reset()
	stderr.Reset()
	err = runWith([]string{"workflow", "submit"}, stdout, stderr)
	if err == nil {
		t.Fatalf("expected error on submit without args")
	}
}

func TestWorkflowSubmitDryRunAndValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid workflow JSON
	validFile := filepath.Join(tmpDir, "valid_workflow.json")
	validJSON := `{
		"workflow_id": "test-submit",
		"steps": {
			"step-1": {"id": "step-1", "command": "echo step1"},
			"step-2": {"id": "step-2", "depends_on": ["step-1"], "command": "echo step2"}
		}
	}`
	if err := os.WriteFile(validFile, []byte(validJSON), 0644); err != nil {
		t.Fatalf("write valid file failed: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Dry run submission
	err := runWith([]string{"workflow", "submit", validFile, "--dry-run"}, stdout, stderr)
	if err != nil {
		t.Fatalf("dry run failed: %v (stderr: %s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "validated successfully") {
		t.Fatalf("unexpected dry-run output: %s", stdout.String())
	}

	// Actual submission
	stdout.Reset()
	stderr.Reset()
	err = runWith([]string{"workflow", "submit", validFile}, stdout, stderr)
	if err != nil {
		t.Fatalf("submission failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "submitted successfully") {
		t.Fatalf("unexpected submit output: %s", stdout.String())
	}

	// Invalid workflow with cycle
	invalidFile := filepath.Join(tmpDir, "cyclic_workflow.json")
	invalidJSON := `{
		"workflow_id": "test-cyclic",
		"steps": {
			"A": {"id": "A", "depends_on": ["B"]},
			"B": {"id": "B", "depends_on": ["A"]}
		}
	}`
	_ = os.WriteFile(invalidFile, []byte(invalidJSON), 0644)

	stdout.Reset()
	stderr.Reset()
	err = runWith([]string{"workflow", "submit", invalidFile}, stdout, stderr)
	if err == nil {
		t.Fatalf("expected validation failure for cyclic workflow")
	}
	if !strings.Contains(stderr.String(), "circular dependency detected") {
		t.Fatalf("expected cyclic error in stderr, got: %s", stderr.String())
	}
}

func TestWorkflowInspectPauseResumeCancel(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Inspect
	err := runWith([]string{"workflow", "inspect", "run-12345", "--no-color"}, stdout, stderr)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "run-12345") || !strings.Contains(stdout.String(), "step-validate") {
		t.Fatalf("unexpected inspect output: %s", stdout.String())
	}

	// Inspect JSON
	stdout.Reset()
	err = runWith([]string{"--output", "json", "workflow", "inspect", "run-12345"}, stdout, stderr)
	if err != nil {
		t.Fatalf("inspect json failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `"run_id":"run-12345"`) {
		t.Fatalf("unexpected inspect JSON: %s", stdout.String())
	}

	// Pause
	stdout.Reset()
	err = runWith([]string{"workflow", "pause", "run-12345", "--reason=upgrades"}, stdout, stderr)
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "paused successfully") {
		t.Fatalf("unexpected pause output: %s", stdout.String())
	}

	// Resume
	stdout.Reset()
	err = runWith([]string{"workflow", "resume", "run-12345"}, stdout, stderr)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "resumed successfully") {
		t.Fatalf("unexpected resume output: %s", stdout.String())
	}

	// Cancel
	stdout.Reset()
	err = runWith([]string{"workflow", "cancel", "run-12345", "--force"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "forcibly cancelled") {
		t.Fatalf("unexpected cancel output: %s", stdout.String())
	}
}

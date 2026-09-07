package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIFullLifecycleIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	wfFile := filepath.Join(tmpDir, "order_workflow.json")
	wfJSON := `{
		"workflow_id": "order-pipeline",
		"steps": {
			"validate": {"id": "validate", "command": "echo validating"},
			"charge":   {"id": "charge", "depends_on": ["validate"], "command": "echo charging"},
			"ship":     {"id": "ship", "depends_on": ["charge"], "command": "echo shipping"}
		}
	}`
	_ = os.WriteFile(wfFile, []byte(wfJSON), 0644)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// 1. Submit workflow with --output=json
	err := runWith([]string{"--output", "json", "workflow", "submit", wfFile, "--param", "env=prod"}, stdout, stderr)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	var submitResp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &submitResp); err != nil {
		t.Fatalf("failed to parse submit json response: %v\nOutput was: %s", err, stdout.String())
	}

	if submitResp["status"] != "SUBMITTED" || submitResp["workflow_id"] != "order-pipeline" {
		t.Fatalf("unexpected submit response: %v", submitResp)
	}
	runID := submitResp["run_id"].(string)

	// 2. Inspect workflow run with global flags
	stdout.Reset()
	err = runWith([]string{"--namespace", "production", "--output", "json", "workflow", "inspect", runID}, stdout, stderr)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	var inspectResp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &inspectResp); err != nil {
		t.Fatalf("failed to parse inspect json: %v", err)
	}
	if inspectResp["run_id"] != runID {
		t.Fatalf("inspect returned wrong run ID: %v (want %s)", inspectResp["run_id"], runID)
	}

	// 3. Pause workflow run
	stdout.Reset()
	err = runWith([]string{"--output", "json", "workflow", "pause", runID, "--reason", "circuit-breaker"}, stdout, stderr)
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	var pauseResp map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &pauseResp)
	if pauseResp["status"] != "PAUSED" || pauseResp["reason"] != "circuit-breaker" {
		t.Fatalf("unexpected pause response: %v", pauseResp)
	}

	// 4. Resume workflow run
	stdout.Reset()
	err = runWith([]string{"--output", "json", "workflow", "resume", runID}, stdout, stderr)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	var resumeResp map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &resumeResp)
	if resumeResp["status"] != "RUNNING" {
		t.Fatalf("unexpected resume response: %v", resumeResp)
	}

	// 5. Cancel workflow run with force
	stdout.Reset()
	err = runWith([]string{"--output", "json", "workflow", "cancel", runID, "--force"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	var cancelResp map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &cancelResp)
	if cancelResp["status"] != "CANCELLED" || cancelResp["force"] != true {
		t.Fatalf("unexpected cancel response: %v", cancelResp)
	}
}

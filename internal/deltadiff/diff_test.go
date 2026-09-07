package deltadiff

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDiffAndApplyRoundTrip(t *testing.T) {
	src := map[string]any{
		"run_id": "run-001",
		"status": "RUNNING",
		"count":  float64(10),
		"metadata": map[string]any{
			"env":     "staging",
			"cluster": "east-1",
		},
	}

	dst := map[string]any{
		"run_id": "run-001",
		"status": "COMPLETED",
		"count":  float64(15),
		"metadata": map[string]any{
			"env":     "staging",
			"cluster": "east-2",
			"version": "v2.0",
		},
		"output": "all-done",
	}

	patch, err := Diff(src, dst)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(patch) == 0 {
		t.Fatalf("expected non-empty patch")
	}

	// Apply patch to src
	applied, err := Apply(src, patch)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	appliedJSON, _ := json.Marshal(applied)
	dstJSON, _ := json.Marshal(dst)

	if string(appliedJSON) != string(dstJSON) {
		t.Fatalf("round-trip mismatch:\ngot:  %s\nwant: %s", string(appliedJSON), string(dstJSON))
	}
}

func TestBinaryPatchRoundTrip(t *testing.T) {
	originalPatch := Patch{
		{Op: OpAdd, Path: "/settings/retry_limit", Value: float64(5)},
		{Op: OpReplace, Path: "/status", Value: "SUCCESS"},
		{Op: OpRemove, Path: "/temporary_token"},
		{Op: OpCopy, Path: "/backup_key", From: "/primary_key"},
		{Op: OpMove, Path: "/archived/item", From: "/active/item"},
		{Op: OpTest, Path: "/version", Value: "1.0"},
	}

	binBytes, err := EncodeBinary(originalPatch)
	if err != nil {
		t.Fatalf("EncodeBinary failed: %v", err)
	}

	if len(binBytes) < 9 {
		t.Fatalf("binary bytes too short: %d", len(binBytes))
	}

	decodedPatch, err := DecodeBinary(binBytes)
	if err != nil {
		t.Fatalf("DecodeBinary failed: %v", err)
	}

	if len(decodedPatch) != len(originalPatch) {
		t.Fatalf("expected %d ops, got %d", len(originalPatch), len(decodedPatch))
	}

	for i := range originalPatch {
		orig := originalPatch[i]
		dec := decodedPatch[i]

		if orig.Op != dec.Op || orig.Path != dec.Path || orig.From != dec.From {
			t.Fatalf("op %d mismatch: got %+v, want %+v", i, dec, orig)
		}
		if orig.Value != nil && !reflect.DeepEqual(orig.Value, dec.Value) {
			t.Fatalf("op %d value mismatch: got %v, want %v", i, dec.Value, orig.Value)
		}
	}
}

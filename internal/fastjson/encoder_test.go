package fastjson

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestStreamEncoderCorrectness(t *testing.T) {
	buf := Acquire()
	defer Release(buf)

	enc := NewStreamEncoder(buf)

	enc.BeginObject()
	enc.Key("title")
	enc.WriteString("Tempest Workflow Engine")
	enc.Key("version")
	enc.WriteInt64(2)
	enc.Key("active")
	enc.WriteBool(true)
	enc.Key("ratio")
	enc.WriteFloat64(0.95)
	enc.Key("null_val")
	enc.WriteNull()
	enc.Key("tags")
	enc.BeginArray()
	enc.WriteString("distributed")
	enc.WriteString("fault-tolerant")
	enc.WriteInt64(42)
	enc.EndArray()
	enc.Key("nested")
	enc.BeginObject()
	enc.Key("inner_key")
	enc.WriteString("inner_val")
	enc.EndObject()
	enc.EndObject()

	jsonBytes := buf.Bytes()

	// Verify it parses cleanly with standard encoding/json
	var parsed map[string]any
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("standard json.Unmarshal failed: %v\nJSON was: %s", err, string(jsonBytes))
	}

	if parsed["title"] != "Tempest Workflow Engine" {
		t.Fatalf("unexpected title: %v", parsed["title"])
	}
	if parsed["version"] != float64(2) {
		t.Fatalf("unexpected version: %v", parsed["version"])
	}
	if parsed["active"] != true {
		t.Fatalf("unexpected active: %v", parsed["active"])
	}
	if parsed["null_val"] != nil {
		t.Fatalf("expected null_val nil, got %v", parsed["null_val"])
	}

	tags := parsed["tags"].([]any)
	if len(tags) != 3 || tags[0] != "distributed" || tags[1] != "fault-tolerant" || tags[2] != float64(42) {
		t.Fatalf("unexpected tags: %v", tags)
	}

	nested := parsed["nested"].(map[string]any)
	if nested["inner_key"] != "inner_val" {
		t.Fatalf("unexpected nested: %v", nested)
	}
}

func TestWorkflowEventFormatting(t *testing.T) {
	payload := map[string]string{
		"step":   "step-deploy",
		"status": "SUCCESS",
	}

	fastBytes := FormatWorkflowEventJSON(101, "StepCompleted", "step-deploy", payload)

	var parsed map[string]any
	if err := json.Unmarshal(fastBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal formatted event: %v", err)
	}

	if parsed["seq"] != float64(101) {
		t.Fatalf("unexpected seq: %v", parsed["seq"])
	}
	if parsed["type"] != "StepCompleted" {
		t.Fatalf("unexpected type: %v", parsed["type"])
	}
	if parsed["step_id"] != "step-deploy" {
		t.Fatalf("unexpected step_id: %v", parsed["step_id"])
	}

	p := parsed["payload"].(map[string]any)
	expectedPayload := map[string]any{
		"step":   "step-deploy",
		"status": "SUCCESS",
	}
	if !reflect.DeepEqual(p, expectedPayload) {
		t.Fatalf("payload mismatch: got %v, want %v", p, expectedPayload)
	}
}

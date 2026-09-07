package fastjson

import (
	"encoding/json"
	"testing"
)

type sampleEvent struct {
	Seq     uint64            `json:"seq"`
	Type    string            `json:"type"`
	StepID  string            `json:"step_id"`
	Payload map[string]string `json:"payload"`
}

func BenchmarkFastWorkflowEventFormat(b *testing.B) {
	payload := map[string]string{
		"action": "run",
		"status": "SUCCESS",
		"host":   "cluster-worker-01",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatWorkflowEventJSON(uint64(i), "StepCompleted", "step-exec", payload)
	}
}

func BenchmarkStdJSONWorkflowEventFormat(b *testing.B) {
	evt := sampleEvent{
		Seq:    100,
		Type:   "StepCompleted",
		StepID: "step-exec",
		Payload: map[string]string{
			"action": "run",
			"status": "SUCCESS",
			"host":   "cluster-worker-01",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(evt)
	}
}

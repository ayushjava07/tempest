package types

import (
	"encoding/json"
	"testing"
)

func FuzzParseRunState(f *testing.F) {
	f.Add("PENDING")
	f.Add("RUNNING")
	f.Add("SUCCEEDED")
	f.Add("FAILED")
	f.Add("CANCELLED")
	f.Add("TIMED_OUT")
	f.Add("bogus")
	f.Add("")
	f.Add("pending")
	f.Add("PENDING ")
	f.Fuzz(func(t *testing.T, state string) {
		result, err := ParseRunState(state)
		if err != nil {
			return
		}
		if !result.IsTerminal() && result != StatePending && result != StateQueued && result != StateRunning {
			t.Errorf("non-terminal non-pending/queued/running state: %s", result)
		}
	})
}

func FuzzParseEventType(f *testing.F) {
	f.Add("run.created")
	f.Add("step.queued")
	f.Add("step.started")
	f.Add("step.completed")
	f.Add("step.failed")
	f.Add("run.completed")
	f.Add("run.failed")
	f.Add("run.cancelled")
	f.Add("run.timed_out")
	f.Add("unknown")
	f.Add("")
	f.Fuzz(func(t *testing.T, eventType string) {
		result, err := ParseEventType(eventType)
		if err != nil {
			return
		}
		if result == "" {
			t.Error("empty event type after successful parse")
		}
		_ = result.Short()
	})
}

func FuzzEventJSON_Roundtrip(f *testing.F) {
	f.Add("ev1", "run.created", 1, "default", "run1", "step1")
	f.Add("ev2", "step.failed", 2, "ns", "", "")
	f.Fuzz(func(t *testing.T, id, typ string, ver int, ns, runID, stepID string) {
		ev := Event{
			ID: id, Type: EventType(typ), Version: ver,
			Namespace: Namespace(ns), RunID: runID, StepID: stepID,
			Payload: map[string]any{"key": "val"},
		}
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		var ev2 Event
		if err := json.Unmarshal(data, &ev2); err != nil {
			t.Fatal(err)
		}
		if ev2.ID != ev.ID {
			t.Errorf("ID mismatch: %s != %s", ev2.ID, ev.ID)
		}
		if ev2.RunID != ev.RunID {
			t.Errorf("RunID mismatch: %s != %s", ev2.RunID, ev.RunID)
		}
	})
}

func FuzzRunJSON_Roundtrip(f *testing.F) {
	f.Add("run1", "default", "wf", 1, "RUNNING")
	f.Add("run2", "ns", "test", 5, "SUCCEEDED")
	f.Fuzz(func(t *testing.T, id, ns, wfName string, wfVer int, state string) {
		run := Run{
			ID: id, Namespace: Namespace(ns),
			Workflow: WorkflowID{Name: wfName, Version: wfVer},
			State:    RunState(state),
			Input:    RunInput{Payload: map[string]any{"k": "v"}},
			CreatedAt: Now(),
		}
		data, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		var run2 Run
		if err := json.Unmarshal(data, &run2); err != nil {
			t.Fatal(err)
		}
		if run2.ID != run.ID {
			t.Errorf("ID mismatch")
		}
	})
}

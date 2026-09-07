package replay

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrNonDeterministicBranch = errors.New("replay: non-deterministic execution branch detected")
	ErrHistoryExhausted       = errors.New("replay: recorded history exhausted prematurely")
	ErrEventMismatch          = errors.New("replay: event type mismatch during replay")
)

// EventType identifies a discrete workflow milestone.
type EventType string

const (
	EventWorkflowStarted   EventType = "WorkflowStarted"
	EventStepScheduled     EventType = "StepScheduled"
	EventStepStarted       EventType = "StepStarted"
	EventStepCompleted     EventType = "StepCompleted"
	EventStepFailed        EventType = "StepFailed"
	EventWorkflowCompleted EventType = "WorkflowCompleted"
)

// HistoryEvent records a single immutable execution event.
type HistoryEvent struct {
	Seq       uint64            `json:"seq"`
	Type      EventType         `json:"type"`
	StepID    string            `json:"step_id,omitempty"`
	Payload   map[string]string `json:"payload,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// History encapsulates a complete chronologically ordered sequence of execution events.
type History struct {
	RunID      string         `json:"run_id"`
	WorkflowID string         `json:"workflow_id"`
	Events     []HistoryEvent `json:"events"`
}

// Recorder collects events during live workflow execution.
type Recorder struct {
	mu      sync.Mutex
	runID   string
	wfID    string
	seq     uint64
	history []HistoryEvent
}

// NewRecorder creates an event recorder.
func NewRecorder(runID, workflowID string) *Recorder {
	return &Recorder{
		runID:   runID,
		wfID:    workflowID,
		history: make([]HistoryEvent, 0),
	}
}

// Record appends a new event with monotonically increasing sequence.
func (r *Recorder) Record(evtType EventType, stepID string, payload map[string]string) HistoryEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	evt := HistoryEvent{
		Seq:       r.seq,
		Type:      evtType,
		StepID:    stepID,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}
	r.history = append(r.history, evt)
	return evt
}

// Snapshot returns the recorded execution history.
func (r *Recorder) Snapshot() History {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]HistoryEvent, len(r.history))
	copy(cp, r.history)

	return History{
		RunID:      r.runID,
		WorkflowID: r.wfID,
		Events:     cp,
	}
}

package replay

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ReplayReport summarizes the outcome of a deterministic history replay.
type ReplayReport struct {
	RunID         string        `json:"run_id"`
	WorkflowID    string        `json:"workflow_id"`
	TotalEvents   int           `json:"total_events"`
	MatchedEvents int           `json:"matched_events"`
	Completed     bool          `json:"completed"`
	Duration      time.Duration `json:"duration"`
}

// Replayer coordinates step-by-step verification against recorded event history.
type Replayer struct {
	mu      sync.Mutex
	history History
	cursor  int
}

// NewReplayer creates an execution replayer initialized with recorded history.
func NewReplayer(h History) *Replayer {
	return &Replayer{
		history: h,
		cursor:  0,
	}
}

// MatchStep verifies that the next event in history corresponds to the expected step action.
func (r *Replayer) MatchStep(expectedType EventType, stepID string) (HistoryEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cursor >= len(r.history.Events) {
		return HistoryEvent{}, fmt.Errorf("%w: step %s (expected %s at index %d)",
			ErrHistoryExhausted, stepID, expectedType, r.cursor)
	}

	actual := r.history.Events[r.cursor]
	if actual.Type != expectedType || (stepID != "" && actual.StepID != stepID) {
		return actual, fmt.Errorf("%w: expected [%s, step=%s], got [%s, step=%s] at index %d",
			ErrEventMismatch, expectedType, stepID, actual.Type, actual.StepID, r.cursor)
	}

	r.cursor++
	return actual, nil
}

// Peek returns the next event without advancing the replay cursor.
func (r *Replayer) Peek() (HistoryEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cursor >= len(r.history.Events) {
		return HistoryEvent{}, false
	}
	return r.history.Events[r.cursor], true
}

// Replay iterates over the recorded history deterministically and invokes the step handler.
func (r *Replayer) Replay(ctx context.Context, handler func(ctx context.Context, evt HistoryEvent) error) (ReplayReport, error) {
	startTime := time.Now()
	report := ReplayReport{
		RunID:       r.history.RunID,
		WorkflowID:  r.history.WorkflowID,
		TotalEvents: len(r.history.Events),
	}

	for {
		select {
		case <-ctx.Done():
			report.Duration = time.Since(startTime)
			return report, ctx.Err()
		default:
		}

		r.mu.Lock()
		if r.cursor >= len(r.history.Events) {
			r.mu.Unlock()
			break
		}
		evt := r.history.Events[r.cursor]
		r.cursor++
		report.MatchedEvents++
		r.mu.Unlock()

		if handler != nil {
			if err := handler(ctx, evt); err != nil {
				report.Duration = time.Since(startTime)
				return report, fmt.Errorf("step handler failed at seq %d (step %s): %w", evt.Seq, evt.StepID, err)
			}
		}
	}

	report.Completed = true
	report.Duration = time.Since(startTime)
	return report, nil
}

// Reset rewinds the replay cursor to the beginning.
func (r *Replayer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cursor = 0
}

// IsComplete returns true if all recorded events have been matched.
func (r *Replayer) IsComplete() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cursor == len(r.history.Events)
}

// RemainingEvents returns the count of unmatched events left in history.
func (r *Replayer) RemainingEvents() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.history.Events) - r.cursor
}

// Cursor returns the current event position.
func (r *Replayer) Cursor() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cursor
}

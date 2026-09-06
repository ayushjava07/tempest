package saga

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSagaAborted           = errors.New("saga: step execution failed, saga aborted")
	ErrCompensationFailed    = errors.New("saga: one or more compensation actions failed")
	ErrStepAlreadyRegistered = errors.New("saga: step ID already registered")
)

type SagaState string

const (
	StatePending              SagaState = "PENDING"
	StateExecuting            SagaState = "EXECUTING"
	StateCompleted            SagaState = "COMPLETED"
	StateCompensating         SagaState = "COMPENSATING"
	StateCompensated          SagaState = "COMPENSATED"
	StatePartiallyCompensated SagaState = "PARTIALLY_COMPENSATED"
)

// Action represents an executable forward operation.
type Action func(ctx context.Context) error

// CompensateAction represents the inverse operation to undo an action.
type CompensateAction func(ctx context.Context) error

// Step defines a step with forward action and reverse compensation.
type Step struct {
	ID         string
	Forward    Action
	Compensate CompensateAction
}

// LogEntry records the outcome of forward steps and compensations.
type LogEntry struct {
	StepID    string
	Action    string // "FORWARD" or "COMPENSATE"
	Success   bool
	Error     string
	Timestamp time.Time
}

// Coordinator orchestrates transactional execution across multiple distributed steps.
type Coordinator struct {
	mu           sync.RWMutex
	id           string
	steps        []Step
	state        SagaState
	executed     []string // IDs of forward-executed steps in order
	logs         []LogEntry
	compensation map[string]CompensateAction
}

func NewCoordinator(id string) *Coordinator {
	return &Coordinator{
		id:           id,
		state:        StatePending,
		compensation: make(map[string]CompensateAction),
	}
}

// AddStep registers a step with forward action and optional compensation.
func (c *Coordinator) AddStep(id string, forward Action, compensate CompensateAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range c.steps {
		if s.ID == id {
			return ErrStepAlreadyRegistered
		}
	}

	c.steps = append(c.steps, Step{
		ID:         id,
		Forward:    forward,
		Compensate: compensate,
	})
	if compensate != nil {
		c.compensation[id] = compensate
	}
	return nil
}

// State returns the current lifecycle state of the saga.
func (c *Coordinator) State() SagaState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Logs returns a copy of the saga's audit log.
func (c *Coordinator) Logs() []LogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]LogEntry, len(c.logs))
	copy(out, c.logs)
	return out
}

// Execute runs all forward steps sequentially, initiating rollback on failure.
func (c *Coordinator) Execute(ctx context.Context) error {
	c.mu.Lock()
	c.state = StateExecuting
	steps := append([]Step(nil), c.steps...)
	c.mu.Unlock()

	for _, s := range steps {
		err := s.Forward(ctx)
		c.mu.Lock()
		if err == nil {
			c.executed = append(c.executed, s.ID)
			c.logs = append(c.logs, LogEntry{
				StepID:    s.ID,
				Action:    "FORWARD",
				Success:   true,
				Timestamp: time.Now().UTC(),
			})
			c.mu.Unlock()
			continue
		}

		// Forward step failed!
		c.logs = append(c.logs, LogEntry{
			StepID:    s.ID,
			Action:    "FORWARD",
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		})
		c.mu.Unlock()

		// Run reverse compensations
		compErr := c.compensate(ctx)
		if compErr != nil {
			return fmt.Errorf("%w: forward error %v; compensation error %v", ErrCompensationFailed, err, compErr)
		}
		return fmt.Errorf("%w: step %s: %v", ErrSagaAborted, s.ID, err)
	}

	c.mu.Lock()
	c.state = StateCompleted
	c.mu.Unlock()
	return nil
}

func (c *Coordinator) compensate(ctx context.Context) error {
	c.mu.Lock()
	c.state = StateCompensating
	executed := append([]string(nil), c.executed...)
	c.mu.Unlock()

	hasFailure := false
	var firstErr error

	// Compensate in reverse order (LIFO)
	for i := len(executed) - 1; i >= 0; i-- {
		stepID := executed[i]
		c.mu.RLock()
		compFn, hasComp := c.compensation[stepID]
		c.mu.RUnlock()

		if !hasComp || compFn == nil {
			continue
		}

		err := compFn(ctx)
		c.mu.Lock()
		if err != nil {
			hasFailure = true
			if firstErr == nil {
				firstErr = err
			}
			c.logs = append(c.logs, LogEntry{
				StepID:    stepID,
				Action:    "COMPENSATE",
				Success:   false,
				Error:     err.Error(),
				Timestamp: time.Now().UTC(),
			})
		} else {
			c.logs = append(c.logs, LogEntry{
				StepID:    stepID,
				Action:    "COMPENSATE",
				Success:   true,
				Timestamp: time.Now().UTC(),
			})
		}
		c.mu.Unlock()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if hasFailure {
		c.state = StatePartiallyCompensated
		return firstErr
	}
	c.state = StateCompensated
	return nil
}

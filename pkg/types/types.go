package types

import (
	"crypto/rand"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"strings"
	"time"
)

func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Namespace string

type WorkflowID struct {
	Namespace Namespace `json:"namespace"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
}

type StepState string

const (
	StepPending   StepState = "PENDING"
	StepQueued    StepState = "QUEUED"
	StepRunning   StepState = "RUNNING"
	StepSucceeded StepState = "SUCCEEDED"
	StepFailed    StepState = "FAILED"
	StepSkipped   StepState = "SKIPPED"
)

type RunState string

const (
	StatePending   RunState = "PENDING"
	StateQueued    RunState = "QUEUED"
	StateRunning   RunState = "RUNNING"
	StateSucceeded RunState = "SUCCEEDED"
	StateFailed    RunState = "FAILED"
	StateCancelled RunState = "CANCELLED"
	StateTimedOut  RunState = "TIMED_OUT"
)

func (s RunState) IsTerminal() bool {
	switch s {
	case StateSucceeded, StateFailed, StateCancelled, StateTimedOut:
		return true
	default:
		return false
	}
}

func ParseRunState(s string) (RunState, error) {
	c := RunState(strings.ToUpper(strings.TrimSpace(s)))
	switch c {
	case StatePending, StateQueued, StateRunning, StateSucceeded, StateFailed, StateCancelled, StateTimedOut:
		return c, nil
	default:
		return "", fmt.Errorf("unknown run state %q", s)
	}
}

type RetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialInterval time.Duration `json:"initial_interval"`
	MaxInterval     time.Duration `json:"max_interval"`
	Multiplier      float64       `json:"multiplier"`
	MaxElapsed      time.Duration `json:"max_elapsed"`
}

type StepDefinition struct {
	ID            string        `json:"id"`
	Handler       string        `json:"handler"`
	DependsOn     []string      `json:"depends_on,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
	Retry         RetryPolicy   `json:"retry,omitempty"`
	InputSchema   string        `json:"input_schema,omitempty"`
	OutputSchema  string        `json:"output_schema,omitempty"`
	IsGateway     bool          `json:"is_gateway,omitempty"`
	ContinueOnErr bool          `json:"continue_on_err,omitempty"`
}

type WorkflowDefinition struct {
	ID          WorkflowID       `json:"id"`
	Description string           `json:"description,omitempty"`
	Steps       []StepDefinition `json:"steps"`
	Tags        map[string]string `json:"tags,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Timeout     time.Duration    `json:"timeout,omitempty"`
}

type RunInput struct {
	Workflow    WorkflowID        `json:"workflow"`
	Payload     map[string]any    `json:"payload"`
	Labels      map[string]string `json:"labels,omitempty"`
	ParentRunID string            `json:"parent_run_id,omitempty"`
}

type StepRun struct {
	StepID      string            `json:"step_id"`
	State       StepState         `json:"state"`
	Attempt     int               `json:"attempt"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	Output      map[string]any    `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	LastErrType string            `json:"last_err_type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type Run struct {
	ID          string            `json:"id"`
	Namespace   Namespace         `json:"namespace"`
	Workflow    WorkflowID        `json:"workflow"`
	State       RunState          `json:"state"`
	Input       RunInput          `json:"input"`
	Steps       []StepRun         `json:"steps"`
	Attempt     int               `json:"attempt"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	Error       string            `json:"error,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	ParentRunID string            `json:"parent_run_id,omitempty"`
}

type QueueItem struct {
	RunID      string    `json:"run_id"`
	Namespace  Namespace `json:"namespace"`
	StepID     string    `json:"step_id"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	VisibleAt  time.Time `json:"visible_at"`
	Attempt    int       `json:"attempt"`
}

type EventType string

const (
	EventRunCreated   EventType = "run.created"
	EventStepQueued   EventType = "step.queued"
	EventStepStarted  EventType = "step.started"
	EventStepDone     EventType = "step.completed"
	EventStepFailed   EventType = "step.failed"
	EventRunCompleted EventType = "run.completed"
	EventRunFailed    EventType = "run.failed"
	EventRunCancelled EventType = "run.cancelled"
	EventRunTimedOut  EventType = "run.timed_out"
	EventRetrySched   EventType = "retry.scheduled"
)

type Event struct {
	ID        string         `json:"id"`
	Type      EventType      `json:"type"`
	Version   int            `json:"version"`
	Namespace Namespace      `json:"namespace"`
	RunID     string         `json:"run_id"`
	StepID    string         `json:"step_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Payload   map[string]any `json:"payload"`
	Attempt   int            `json:"attempt"`
}

func (t EventType) Short() string {
	switch t {
	case EventRunCreated:
		return "run-created"
	case EventStepQueued:
		return "step-queued"
	case EventStepStarted:
		return "step-started"
	case EventStepDone:
		return "step-done"
	case EventStepFailed:
		return "step-failed"
	case EventRunCompleted:
		return "run-completed"
	case EventRunFailed:
		return "run-failed"
	case EventRunCancelled:
		return "run-cancelled"
	case EventRunTimedOut:
		return "run-timedout"
	case EventRetrySched:
		return "retry-scheduled"
	default:
		return "event"
	}
}

func ParseEventType(s string) (EventType, error) {
	c := EventType(strings.TrimSpace(s))
	switch c {
	case EventRunCreated, EventStepQueued, EventStepStarted, EventStepDone,
		EventStepFailed, EventRunCompleted, EventRunFailed, EventRunCancelled,
		EventRunTimedOut, EventRetrySched:
		return c, nil
	default:
		return "", fmt.Errorf("unknown event type %q", s)
	}
}

type WebhookEndpoint struct {
	ID        string        `json:"id"`
	Namespace Namespace     `json:"namespace"`
	URL       string        `json:"url"`
	Secret    string        `json:"secret,omitempty"`
	Types     []EventType   `json:"types"`
	Active    bool          `json:"active"`
	Timeout   time.Duration `json:"timeout,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

type Delivery struct {
	ID          string         `json:"id"`
	EventID     string         `json:"event_id"`
	EndpointID  string         `json:"endpoint_id"`
	Namespace   Namespace      `json:"namespace"`
	Status      DeliveryStatus `json:"status"`
	Attempts    int            `json:"attempts"`
	LastStatus  int            `json:"last_status"`
	LastError   string         `json:"last_error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	AttemptedAt *time.Time     `json:"attempted_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
}

type APIToken struct {
	ID        string     `json:"id"`
	Namespace Namespace  `json:"namespace"`
	Role      string     `json:"role"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	TokenHash string     `json:"-"`
}

type Submission struct {
	RunID    string    `json:"run_id"`
	State    RunState  `json:"state"`
	Received time.Time `json:"received"`
}

var _ = stderrors.New("placeholder")

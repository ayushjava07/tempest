package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	stderrors "errors"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
	"github.com/tempest-io/tempest/internal/plugin"
	"github.com/tempest-io/tempest/internal/workflow"
	"github.com/tempest-io/tempest/pkg/errors"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type Clock func() time.Time

type EventSink interface {
	Publish(ctx context.Context, e *ttypes.Event) error
}

type Metrics interface {
	Incr(name string)
	Observe(name string, value float64)
}

func EmptyMetrics() Metrics { return &noopMetrics{} }

type noopMetrics struct{}

func (n *noopMetrics) Incr(string)          {}
func (n *noopMetrics) Observe(string, float64) {}

type Log interface {
	Errorf(format string, args ...any)
	Warnf(format string, args ...any)
	Infof(format string, args ...any)
}

type QuietLog struct{}

func (QuietLog) Errorf(string, ...any) {}
func (QuietLog) Warnf(string, ...any)  {}
func (QuietLog) Infof(string, ...any)  {}

func MustNotFound(err error) bool { return errors.Is(err, errors.ErrNotFound) }

type Options struct {
	Clock      Clock
	MaxDequeue int
	LeaseTTL   time.Duration
	EventSink  EventSink
	Metrics    Metrics
	Log        Log
}

type Engine struct {
	store      persistence.Store
	registry   *plugin.Registry
	now        Clock
	events     EventSink
	metrics    Metrics
	log        Log
	maxDequeue int
}

func NewEngine(store persistence.Store, registry *plugin.Registry, opts Options) *Engine {
	o := opts
	if o.Clock == nil {
		o.Clock = time.Now
	}
	if o.MaxDequeue <= 0 {
		o.MaxDequeue = 32
	}
	if o.LeaseTTL <= 0 {
		o.LeaseTTL = 30 * time.Second
	}
	if o.Metrics == nil {
		o.Metrics = EmptyMetrics()
	}
	if o.Log == nil {
		o.Log = QuietLog{}
	}
	return &Engine{
		store:      store,
		registry:   registry,
		now:        o.Clock,
		events:     o.EventSink,
		metrics:    o.Metrics,
		log:        o.Log,
		maxDequeue: o.MaxDequeue,
	}
}

func (e *Engine) Submit(ctx context.Context, input ttypes.RunInput) (string, error) {
	now := e.now()
	def, err := e.store.GetDefinitionByName(ctx, input.Workflow.Namespace, input.Workflow.Name)
	if err != nil {
		return "", err
	}
	input.Workflow = def.ID
	run, err := workflow.NewRun(def, input, now)
	if err != nil {
		return "", err
	}
	if err := e.store.CreateRun(ctx, run); err != nil {
		return "", err
	}
	if err := e.store.UpdateRunState(ctx, run.Namespace, run.ID, ttypes.StateQueued, now); err != nil {
		return "", err
	}
	if err := e.enqueueReadySteps(ctx, def, run); err != nil {
		return "", err
	}
	e.emit(ctx, ttypes.Event{
		Type: ttypes.EventRunCreated, Version: 1,
		Namespace: run.Namespace, RunID: run.ID,
		CreatedAt: now, Payload: map[string]any{"workflow": def.ID.Name, "version": def.ID.Version},
	})
	e.metrics.Incr("runs.submitted")
	return run.ID, nil
}

func (e *Engine) enqueueReadySteps(ctx context.Context, def *ttypes.WorkflowDefinition, run *ttypes.Run) error {
	now := e.now()
	for _, s := range workflow.ReadySteps(def, run) {
		item := queueItemFor(run, s, now, now)
		if err := e.store.Enqueue(ctx, item); err != nil {
			return err
		}
		for i := range run.Steps {
			if run.Steps[i].StepID == s.StepID {
				run.Steps[i].State = ttypes.StepQueued
				_ = e.store.UpdateStepState(ctx, run.Namespace, run.ID, run.Steps[i])
				break
			}
		}
		e.emit(ctx, ttypes.Event{
			Type: ttypes.EventStepQueued, Version: 1,
			Namespace: run.Namespace, RunID: run.ID, StepID: s.StepID,
			CreatedAt: now, Attempt: s.Attempt + 1,
		})
	}
	return nil
}

func queueItemFor(run *ttypes.Run, step ttypes.StepRun, now, visibleAt time.Time) ttypes.QueueItem {
	return ttypes.QueueItem{
		RunID: run.ID, Namespace: run.Namespace, StepID: step.StepID,
		EnqueuedAt: now, VisibleAt: visibleAt, Attempt: step.Attempt + 1,
	}
}

func (e *Engine) ProcessLeases(ctx context.Context) error {
	leases, err := e.dequeueLeases(ctx)
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if err := e.processLease(ctx, lease); err != nil {
			e.log.Warnf("process lease: %v", err)
		}
	}
	return nil
}

func (e *Engine) dequeueLeases(ctx context.Context) ([]persistence.QueueLease, error) {
	return e.store.DequeueAny(ctx, e.maxDequeue, e.now())
}

func (e *Engine) processLease(ctx context.Context, lease persistence.QueueLease) error {
	return e.executeLease(ctx, lease)
}

func (e *Engine) executeLease(ctx context.Context, lease persistence.QueueLease) error {
	run, err := e.store.GetRun(ctx, lease.Item.Namespace, lease.Item.RunID)
	if err != nil {
		return err
	}
	def, err := e.store.GetDefinition(ctx, run.Workflow)
	if err != nil {
		return err
	}
	handler, err := e.registry.Resolve(findStepDef(def, lease.Item.StepID).Handler)
	if err != nil {
		return e.failStep(ctx, run, def, lease, err)
	}
	_ = e.markStepRunning(ctx, run, lease.Item, e.now())
	e.emit(ctx, ttypes.Event{
		Type: ttypes.EventStepStarted, Version: 1,
		Namespace: run.Namespace, RunID: run.ID, StepID: lease.Item.StepID,
		CreatedAt: e.now(), Attempt: lease.Item.Attempt,
	})
	result, execErr := handler.Execute(ctx, plugin.Handle{
		RunID: run.ID, StepID: lease.Item.StepID,
		RunInput: run.Input.Payload,
		Deadline: findStepDef(def, lease.Item.StepID).Timeout,
		Now:      e.now,
	})
	if execErr != nil {
		return e.failStep(ctx, run, def, lease, execErr)
	}
	if result.Error != nil {
		return e.failStep(ctx, run, def, lease, result.Error)
	}
	return e.completeStep(ctx, run, def, lease, result.Output)
}

func (e *Engine) markStepRunning(ctx context.Context, run *ttypes.Run, item ttypes.QueueItem, at time.Time) error {
	for i, s := range run.Steps {
		if s.StepID == item.StepID {
			run.Steps[i].State = ttypes.StepRunning
			run.Steps[i].StartedAt = &at
			return e.store.UpdateStepState(ctx, run.Namespace, run.ID, run.Steps[i])
		}
	}
	return nil
}

func (e *Engine) completeStep(ctx context.Context, run *ttypes.Run, def *ttypes.WorkflowDefinition, lease persistence.QueueLease, output map[string]any) error {
	now := e.now()
	if err := e.store.CompleteStep(ctx, lease.Item.Namespace, lease.Item.RunID, lease.Item.StepID, lease.Token); err != nil {
		return err
	}
	for i, s := range run.Steps {
		if s.StepID == lease.Item.StepID {
			run.Steps[i].State = ttypes.StepSucceeded
			run.Steps[i].FinishedAt = &now
			run.Steps[i].Output = output
			_ = e.store.UpdateStepState(ctx, run.Namespace, run.ID, run.Steps[i])
			break
		}
	}
	e.emit(ctx, ttypes.Event{
		Type: ttypes.EventStepDone, Version: 1,
		Namespace: run.Namespace, RunID: run.ID, StepID: lease.Item.StepID,
		CreatedAt: now, Attempt: lease.Item.Attempt,
	})
	allDone := true
	for _, s := range run.Steps {
		if s.State != ttypes.StepSucceeded && s.State != ttypes.StepSkipped {
			allDone = false
			break
		}
	}
	if allDone {
		return e.promoteRun(ctx, run)
	}
	if err := e.enqueueReadySteps(ctx, def, run); err != nil {
		return err
	}
	return nil
}

func (e *Engine) failStep(ctx context.Context, run *ttypes.Run, def *ttypes.WorkflowDefinition, lease persistence.QueueLease, err error) error {
	now := e.now()
	msg := err.Error()
	if msg == "" {
		msg = "step failed"
	}
	_ = e.store.UpdateStepState(ctx, lease.Item.Namespace, lease.Item.RunID, ttypes.StepRun{
		StepID: lease.Item.StepID, State: ttypes.StepFailed,
		Error: msg, Attempt: lease.Item.Attempt,
	})
	e.emit(ctx, ttypes.Event{
		Type: ttypes.EventStepFailed, Version: 1,
		Namespace: run.Namespace, RunID: run.ID, StepID: lease.Item.StepID,
		CreatedAt: now, Attempt: lease.Item.Attempt, Payload: map[string]any{"error": msg},
	})
	_ = e.store.CompleteStep(ctx, lease.Item.Namespace, lease.Item.RunID, lease.Item.StepID, lease.Token)
	return e.finishRun(ctx, run, ttypes.StateFailed, msg)
}

func (e *Engine) finishRun(ctx context.Context, run *ttypes.Run, state ttypes.RunState, errMsg string) error {
	now := e.now()
	_ = e.store.UpdateRunState(ctx, run.Namespace, run.ID, state, now)
	if errMsg != "" {
		_ = e.store.UpdateRunError(ctx, run.Namespace, run.ID, errMsg)
	}
	evType := ttypes.EventRunCompleted
	switch state {
	case ttypes.StateFailed:
		evType = ttypes.EventRunFailed
	case ttypes.StateCancelled:
		evType = ttypes.EventRunCancelled
	case ttypes.StateTimedOut:
		evType = ttypes.EventRunTimedOut
	}
	e.emit(ctx, ttypes.Event{
		Type: evType, Version: 1,
		Namespace: run.Namespace, RunID: run.ID,
		CreatedAt: now, Payload: map[string]any{"error": errMsg},
	})
	return nil
}

func (e *Engine) Cancel(ctx context.Context, namespace ttypes.Namespace, runID string) error {
	run, err := e.store.GetRun(ctx, namespace, runID)
	if err != nil {
		return err
	}
	if run.State.IsTerminal() {
		return errors.ConflictError(errors.ErrConflict)
	}
	return e.finishRun(ctx, run, ttypes.StateCancelled, "")
}

func (e *Engine) promoteRun(ctx context.Context, run *ttypes.Run) error {
	return e.finishRun(ctx, run, ttypes.StateSucceeded, "")
}

func (e *Engine) emit(ctx context.Context, ev ttypes.Event) {
	if e.events == nil {
		return
	}
	ev.ID = eventID(ev)
	if err := e.events.Publish(ctx, &ev); err != nil {
		e.log.Warnf("publish event %s: %v", ev.ID, err)
	}
}

func eventID(ev ttypes.Event) string {
	digest := make([]byte, 8)
	_, _ = rand.Read(digest)
	return ev.Type.Short() + "-" + hex.EncodeToString(digest)
}

func findStepDef(d *ttypes.WorkflowDefinition, id string) *ttypes.StepDefinition {
	for i := range d.Steps {
		if d.Steps[i].ID == id {
			return &d.Steps[i]
		}
	}
	return nil
}

type permanentFailure struct{ inner error }

func permanentStepFailure(cause error) error {
	return &permanentFailure{inner: cause}
}

func (p *permanentFailure) Error() string { return p.inner.Error() }
func (p *permanentFailure) Unwrap() error { return p.inner }

func isPermanentFailure(err error) bool {
	var p *permanentFailure
	return stderrors.As(err, &p)
}

package hook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

var (
	ErrHookTimeout = errors.New("hook: execution timeout exceeded")
	ErrNilHook     = errors.New("hook: cannot register nil hook")
)

// Hook defines the lifecycle callback interface for workflow execution events.
type Hook interface {
	Name() string
	OnWorkflowStarted(ctx context.Context, run *ttypes.Run) error
	OnStepCompleted(ctx context.Context, run *ttypes.Run, step *ttypes.StepRun) error
	OnWorkflowFinished(ctx context.Context, run *ttypes.Run) error
}

// HookFilter determines if a hook should be triggered for a specific run.
type HookFilter struct {
	Namespaces    []string
	WorkflowNames []string
	FailuresOnly  bool
}

func (f HookFilter) Matches(run *ttypes.Run) bool {
	if len(f.Namespaces) > 0 {
		matched := false
		for _, ns := range f.Namespaces {
			if string(run.Namespace) == ns {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(f.WorkflowNames) > 0 {
		matched := false
		for _, w := range f.WorkflowNames {
			if run.Workflow.Name == w {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if f.FailuresOnly && run.State != ttypes.StateFailed && run.State != ttypes.StateTimedOut {
		return false
	}

	return true
}

type registeredHook struct {
	hook    Hook
	filter  HookFilter
	async   bool
	timeout time.Duration
}

// Registry manages and dispatches workflow lifecycle hooks.
type Registry struct {
	mu             sync.RWMutex
	hooks          []registeredHook
	asyncWg        sync.WaitGroup
	defaultTimeout time.Duration
	closed         atomic.Bool
}

func NewRegistry() *Registry {
	return &Registry{
		defaultTimeout: 5 * time.Second,
	}
}

// Register adds a synchronous or asynchronous hook to the registry.
func (r *Registry) Register(h Hook, filter HookFilter, async bool, timeout time.Duration) error {
	if h == nil {
		return ErrNilHook
	}
	if timeout <= 0 {
		timeout = r.defaultTimeout
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = append(r.hooks, registeredHook{
		hook:    h,
		filter:  filter,
		async:   async,
		timeout: timeout,
	})
	return nil
}

// NotifyWorkflowStarted notifies all matching hooks of run start.
func (r *Registry) NotifyWorkflowStarted(ctx context.Context, run *ttypes.Run) []error {
	return r.dispatch(ctx, run, func(ctx context.Context, h Hook) error {
		return h.OnWorkflowStarted(ctx, run)
	})
}

// NotifyStepCompleted notifies all matching hooks of a finished step.
func (r *Registry) NotifyStepCompleted(ctx context.Context, run *ttypes.Run, step *ttypes.StepRun) []error {
	return r.dispatch(ctx, run, func(ctx context.Context, h Hook) error {
		return h.OnStepCompleted(ctx, run, step)
	})
}

// NotifyWorkflowFinished notifies all matching hooks of run completion.
func (r *Registry) NotifyWorkflowFinished(ctx context.Context, run *ttypes.Run) []error {
	return r.dispatch(ctx, run, func(ctx context.Context, h Hook) error {
		return h.OnWorkflowFinished(ctx, run)
	})
}

func (r *Registry) dispatch(ctx context.Context, run *ttypes.Run, fn func(context.Context, Hook) error) []error {
	r.mu.RLock()
	hooks := append([]registeredHook(nil), r.hooks...)
	r.mu.RUnlock()

	var syncErrors []error
	var errMu sync.Mutex

	for _, entry := range hooks {
		if !entry.filter.Matches(run) {
			continue
		}

		h := entry.hook
		t := entry.timeout

		if entry.async {
			if r.closed.Load() {
				continue
			}
			r.asyncWg.Add(1)
			go func(hook Hook, timeout time.Duration) {
				defer r.asyncWg.Done()
				execCtx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				_ = fn(execCtx, hook)
			}(h, t)
		} else {
			execCtx, cancel := context.WithTimeout(ctx, t)
			err := fn(execCtx, h)
			cancel()
			if err != nil {
				errMu.Lock()
				syncErrors = append(syncErrors, fmt.Errorf("hook %s: %w", h.Name(), err))
				errMu.Unlock()
			}
		}
	}

	return syncErrors
}

// Close gracefully waits for asynchronous background hooks to complete.
func (r *Registry) Close() {
	if r.closed.CompareAndSwap(false, true) {
		r.asyncWg.Wait()
	}
}

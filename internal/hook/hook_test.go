package hook

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type testHook struct {
	name             string
	workflowStarted  int
	stepCompleted    int
	workflowFinished int
	mu               sync.Mutex
	delay            time.Duration
	fail             bool
}

func newTestHook(name string) *testHook {
	return &testHook{name: name}
}

func (h *testHook) Name() string { return h.name }

func (h *testHook) OnWorkflowStarted(ctx context.Context, run *ttypes.Run) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.delay > 0 {
		select {
		case <-time.After(h.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if h.fail {
		return errors.New("simulated hook failure")
	}
	h.workflowStarted++
	return nil
}

func (h *testHook) OnStepCompleted(ctx context.Context, run *ttypes.Run, step *ttypes.StepRun) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stepCompleted++
	return nil
}

func (h *testHook) OnWorkflowFinished(ctx context.Context, run *ttypes.Run) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workflowFinished++
	return nil
}

func TestHook_SyncNotification(t *testing.T) {
	reg := NewRegistry()
	defer reg.Close()

	h := newTestHook("sync-hook")
	if err := reg.Register(h, HookFilter{}, false, 1*time.Second); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	run := &ttypes.Run{
		ID:        "run-1",
		Namespace: "default",
		State:     ttypes.StateRunning,
	}

	errs := reg.NotifyWorkflowStarted(context.Background(), run)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	h.mu.Lock()
	started := h.workflowStarted
	h.mu.Unlock()
	if started != 1 {
		t.Errorf("expected 1 started notification, got %d", started)
	}
}

func TestHook_AsyncNotification(t *testing.T) {
	reg := NewRegistry()
	defer reg.Close()

	h := newTestHook("async-hook")
	_ = reg.Register(h, HookFilter{}, true, 1*time.Second)

	run := &ttypes.Run{
		ID:        "run-2",
		Namespace: "default",
		State:     ttypes.StateRunning,
	}

	_ = reg.NotifyWorkflowStarted(context.Background(), run)

	// Wait for async execution
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		count := h.workflowStarted
		h.mu.Unlock()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.workflowStarted != 1 {
		t.Errorf("expected 1 async notification, got %d", h.workflowStarted)
	}
}

func TestHook_FilterMatching(t *testing.T) {
	reg := NewRegistry()
	defer reg.Close()

	h := newTestHook("filtered-hook")
	_ = reg.Register(h, HookFilter{
		Namespaces:   []string{"prod"},
		FailuresOnly: true,
	}, false, 1*time.Second)

	// Non-matching run (namespace=default)
	run1 := &ttypes.Run{
		ID:        "run-1",
		Namespace: "default",
		State:     ttypes.StateFailed,
	}
	_ = reg.NotifyWorkflowFinished(context.Background(), run1)
	if h.workflowFinished != 0 {
		t.Errorf("hook should not match default namespace")
	}

	// Non-matching run (state=SUCCEEDED)
	run2 := &ttypes.Run{
		ID:        "run-2",
		Namespace: "prod",
		State:     ttypes.StateSucceeded,
	}
	_ = reg.NotifyWorkflowFinished(context.Background(), run2)
	if h.workflowFinished != 0 {
		t.Errorf("hook should not match succeeded run")
	}

	// Matching run (prod + FAILED)
	run3 := &ttypes.Run{
		ID:        "run-3",
		Namespace: "prod",
		State:     ttypes.StateFailed,
	}
	_ = reg.NotifyWorkflowFinished(context.Background(), run3)
	if h.workflowFinished != 1 {
		t.Errorf("hook should match prod failed run")
	}
}

func TestHook_TimeoutIsolation(t *testing.T) {
	reg := NewRegistry()
	defer reg.Close()

	hSlow := newTestHook("slow-hook")
	hSlow.delay = 200 * time.Millisecond

	// Hook timeout of 30ms should cancel slow hook
	_ = reg.Register(hSlow, HookFilter{}, false, 30*time.Millisecond)

	run := &ttypes.Run{ID: "run-slow"}
	errs := reg.NotifyWorkflowStarted(context.Background(), run)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for timed out hook, got %d", len(errs))
	}
}

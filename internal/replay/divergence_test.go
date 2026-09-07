package replay

import (
	"errors"
	"testing"
)

func TestSideEffectInterceptor(t *testing.T) {
	// 1. Live recording phase
	recInterceptor := NewSideEffectInterceptor(false)

	callCount := 0
	val1, err := recInterceptor.Intercept("gen-uuid-1", func() (string, error) {
		callCount++
		return "uuid-abc-123", nil
	})
	if err != nil || val1 != "uuid-abc-123" {
		t.Fatalf("unexpected intercept result: %s, err: %v", val1, err)
	}

	val2, err := recInterceptor.Intercept("timestamp-seed", func() (string, error) {
		callCount++
		return "1609459200", nil
	})
	if err != nil || val2 != "1609459200" {
		t.Fatalf("unexpected intercept result: %s, err: %v", val2, err)
	}

	if callCount != 2 {
		t.Fatalf("expected fn to be called 2 times, got %d", callCount)
	}

	effects, order := recInterceptor.SideEffects()
	if len(effects) != 2 || len(order) != 2 {
		t.Fatalf("expected 2 side effects recorded")
	}

	// 2. Replay phase
	replayInterceptor := NewSideEffectInterceptor(true)
	replayInterceptor.LoadRecordedSideEffects(effects, order)

	// In replay mode, the lambda should NOT be called; recorded value must be returned
	replayCallCount := 0
	repVal1, err := replayInterceptor.Intercept("gen-uuid-1", func() (string, error) {
		replayCallCount++
		return "different-uuid-unexpected", nil
	})
	if err != nil || repVal1 != "uuid-abc-123" {
		t.Fatalf("expected recorded val uuid-abc-123, got %s (err: %v)", repVal1, err)
	}
	if replayCallCount != 0 {
		t.Fatalf("replayer should not invoke side-effect generator, callCount=%d", replayCallCount)
	}

	// Missing side-effect lookup during replay should trigger non-deterministic error
	_, err = replayInterceptor.Intercept("unknown-side-effect", func() (string, error) {
		return "val", nil
	})
	if !errors.Is(err, ErrNonDeterministicBranch) {
		t.Fatalf("expected ErrNonDeterministicBranch on unknown side effect, got %v", err)
	}
}

func TestBranchDetectorDivergence(t *testing.T) {
	rec := NewRecorder("run-div", "wf-branch")
	rec.Record(EventWorkflowStarted, "", nil)
	rec.Record(EventStepScheduled, "step-check", map[string]string{"branch": "fast-path"})
	rec.Record(EventStepCompleted, "step-check", map[string]string{"result": "ok"})

	history := rec.Snapshot()
	replayer := NewReplayer(history)

	detector := NewBranchDetector(replayer)

	// Step past WorkflowStarted
	_, err := replayer.MatchStep(EventWorkflowStarted, "")
	if err != nil {
		t.Fatalf("failed match: %v", err)
	}

	// Matching branch should succeed
	err = detector.VerifyBranch("step-check", "fast-path")
	if err != nil {
		t.Fatalf("expected valid branch to verify successfully, got: %v", err)
	}

	// Divergent branch should fail with ErrNonDeterministicBranch
	err = detector.VerifyBranch("step-check", "slow-path")
	if !errors.Is(err, ErrNonDeterministicBranch) {
		t.Fatalf("expected ErrNonDeterministicBranch for wrong branch, got %v", err)
	}

	// Step mismatch should fail
	err = detector.VerifyBranch("unexpected-step", "fast-path")
	if !errors.Is(err, ErrNonDeterministicBranch) {
		t.Fatalf("expected ErrNonDeterministicBranch for wrong step, got %v", err)
	}

	divs := detector.Divergences()
	if len(divs) != 2 {
		t.Fatalf("expected 2 recorded divergences, got %d", len(divs))
	}
}

func TestStateDiffGeneration(t *testing.T) {
	original := map[string]string{
		"user_id":  "42",
		"status":   "ACTIVE",
		"attempts": "1",
	}

	// Replayed with:
	// - modified: attempts ("1" -> "2")
	// - removed:  status
	// - added:    new_key
	replayed := map[string]string{
		"user_id":  "42",
		"attempts": "2",
		"new_key":  "surprise",
	}

	diff := DiffState("step-mutate", original, replayed)
	if !diff.HasDivergence() {
		t.Fatalf("expected divergence detected in state diff")
	}

	if len(diff.Modified) != 1 || diff.Modified[0].Key != "attempts" {
		t.Fatalf("expected modified key attempts, got %v", diff.Modified)
	}
	if diff.Modified[0].Original != "1" || diff.Modified[0].Replayed != "2" {
		t.Fatalf("unexpected modified values: %v", diff.Modified[0])
	}

	if len(diff.Removed) != 1 || diff.Removed[0].Key != "status" {
		t.Fatalf("expected removed key status, got %v", diff.Removed)
	}

	if len(diff.Added) != 1 || diff.Added[0].Key != "new_key" {
		t.Fatalf("expected added key new_key, got %v", diff.Added)
	}

	summary := diff.Summary()
	if summary == "" {
		t.Fatalf("summary should not be empty")
	}

	// Test identical states
	diffIdentical := DiffState("step-same", original, original)
	if diffIdentical.HasDivergence() {
		t.Fatalf("identical states should have no divergence")
	}
}

func TestStepMutationTracker(t *testing.T) {
	tracker := NewStepMutationTracker()

	tracker.TrackRead("step-1", "user_id")
	tracker.TrackWrite("step-1", "cache_hit", "false", "true")
	tracker.TrackDelete("step-1", "temp_token", "secret123")
	tracker.TrackRead("step-2", "cache_hit")

	all := tracker.AllMutations()
	if len(all) != 4 {
		t.Fatalf("expected 4 total mutations, got %d", len(all))
	}

	s1Mutations := tracker.MutationsForStep("step-1")
	if len(s1Mutations) != 3 {
		t.Fatalf("expected 3 mutations for step-1, got %d", len(s1Mutations))
	}
	if s1Mutations[0].Type != MutationRead || s1Mutations[1].Type != MutationWrite || s1Mutations[2].Type != MutationDelete {
		t.Fatalf("unexpected mutation types for step-1")
	}
}

package versioning

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestVersioning_SemVerParsingAndCompare(t *testing.T) {
	v1, err := ParseSemVer("v1.2.3")
	if err != nil {
		t.Fatalf("ParseSemVer failed: %v", err)
	}
	if v1.Major != 1 || v1.Minor != 2 || v1.Patch != 3 {
		t.Errorf("unexpected semver fields: %v", v1)
	}
	if v1.String() != "1.2.3" {
		t.Errorf("unexpected String(): %s", v1.String())
	}

	v2, _ := ParseSemVer("1.3.0")
	if v1.Compare(v2) != -1 {
		t.Errorf("expected v1 < v2")
	}
	if v2.Compare(v1) != 1 {
		t.Errorf("expected v2 > v1")
	}

	v3, _ := ParseSemVer("1.2.3")
	if v1.Compare(v3) != 0 {
		t.Errorf("expected v1 == v3")
	}

	// Invalid versions
	invalidVersions := []string{"", "1", "1.2", "1.2.a", "-1.0.0"}
	for _, inv := range invalidVersions {
		_, err := ParseSemVer(inv)
		if !errors.Is(err, ErrInvalidSemVer) {
			t.Errorf("expected ErrInvalidSemVer for %q, got %v", inv, err)
		}
	}
}

func TestVersioning_DiffIdentical(t *testing.T) {
	def1 := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 1},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "echo", Timeout: 10 * time.Second},
		},
	}
	def2 := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 1},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "echo", Timeout: 10 * time.Second},
		},
	}

	diff := CompareDefinitions(def1, def2)
	if diff.Compatibility != LevelIdentical {
		t.Errorf("expected LevelIdentical, got %v", diff.Compatibility)
	}
	if len(diff.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(diff.Changes))
	}
}

func TestVersioning_DiffBreaking(t *testing.T) {
	defOld := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 1},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "echo"},
			{ID: "step-2", Handler: "shell", DependsOn: []string{"step-1"}},
		},
	}

	// Case 1: Step removed
	defRemoved := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 2},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "echo"},
		},
	}
	diff := CompareDefinitions(defOld, defRemoved)
	if diff.Compatibility != LevelBreaking {
		t.Errorf("expected LevelBreaking for removed step, got %v", diff.Compatibility)
	}

	// Case 2: Handler changed
	defHandlerChanged := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 2},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "http"}, // was echo
			{ID: "step-2", Handler: "shell", DependsOn: []string{"step-1"}},
		},
	}
	diff2 := CompareDefinitions(defOld, defHandlerChanged)
	if diff2.Compatibility != LevelBreaking {
		t.Errorf("expected LevelBreaking for handler change, got %v", diff2.Compatibility)
	}
}

func TestVersioning_DiffBackwardsCompatible(t *testing.T) {
	defOld := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 1},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "echo", Timeout: 5 * time.Second},
		},
	}

	// New step added and timeout relaxed
	defNew := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{Name: "wf-1", Version: 2},
		Steps: []ttypes.StepDefinition{
			{ID: "step-1", Handler: "echo", Timeout: 10 * time.Second},
			{ID: "step-2", Handler: "pass"},
		},
	}

	diff := CompareDefinitions(defOld, defNew)
	if diff.Compatibility != LevelBackwardsCompatible {
		t.Errorf("expected LevelBackwardsCompatible, got %v", diff.Compatibility)
	}
	if len(diff.Changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(diff.Changes))
	}
}

func TestVersioning_DecideUpgradeStrategy(t *testing.T) {
	breakingDiff := DiffReport{Compatibility: LevelBreaking}
	compatDiff := DiffReport{Compatibility: LevelBackwardsCompatible}
	identDiff := DiffReport{Compatibility: LevelIdentical}

	// Running workflows with breaking changes must pin
	if strat := DecideUpgradeStrategy(breakingDiff, ttypes.StateRunning); strat != StrategyPinToVersion {
		t.Errorf("expected StrategyPinToVersion, got %s", strat)
	}

	// Running workflows with backwards-compatible changes should drain and exit
	if strat := DecideUpgradeStrategy(compatDiff, ttypes.StateRunning); strat != StrategyDrainAndExit {
		t.Errorf("expected StrategyDrainAndExit, got %s", strat)
	}

	// Pending or non-running workflows auto-migrate
	if strat := DecideUpgradeStrategy(breakingDiff, ttypes.StatePending); strat != StrategyAutoMigrate {
		t.Errorf("expected StrategyAutoMigrate for pending, got %s", strat)
	}
	if strat := DecideUpgradeStrategy(identDiff, ttypes.StateRunning); strat != StrategyAutoMigrate {
		t.Errorf("expected StrategyAutoMigrate for identical, got %s", strat)
	}
}

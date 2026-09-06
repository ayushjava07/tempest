package versioning

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

var (
	ErrInvalidSemVer = errors.New("versioning: invalid semantic version format")
)

// SemVer represents a semantic version (Major.Minor.Patch).
type SemVer struct {
	Major int
	Minor int
	Patch int
}

func ParseSemVer(s string) (SemVer, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return SemVer{}, fmt.Errorf("%w: %q must have 3 segments (x.y.z)", ErrInvalidSemVer, s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return SemVer{}, fmt.Errorf("%w: invalid major %q", ErrInvalidSemVer, parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return SemVer{}, fmt.Errorf("%w: invalid minor %q", ErrInvalidSemVer, parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return SemVer{}, fmt.Errorf("%w: invalid patch %q", ErrInvalidSemVer, parts[2])
	}

	return SemVer{Major: major, Minor: minor, Patch: patch}, nil
}

func (v SemVer) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v SemVer) Compare(other SemVer) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// CompatibilityLevel defines whether two workflow definitions are compatible.
type CompatibilityLevel string

const (
	LevelIdentical           CompatibilityLevel = "IDENTICAL"
	LevelFullyCompatible     CompatibilityLevel = "FULLY_COMPATIBLE"
	LevelBackwardsCompatible CompatibilityLevel = "BACKWARDS_COMPATIBLE"
	LevelBreaking            CompatibilityLevel = "BREAKING"
)

// ChangeKind describes specific differences between versions.
type ChangeKind string

const (
	ChangeStepAdded      ChangeKind = "STEP_ADDED"
	ChangeStepRemoved    ChangeKind = "STEP_REMOVED"
	ChangeHandlerChanged ChangeKind = "HANDLER_CHANGED"
	ChangeDepsChanged    ChangeKind = "DEPS_CHANGED"
	ChangeTimeoutChanged ChangeKind = "TIMEOUT_CHANGED"
)

// SchemaChange represents an atomic difference between two definitions.
type SchemaChange struct {
	Kind        ChangeKind
	StepID      string
	Description string
	IsBreaking  bool
}

// DiffReport summarizes schema comparison results.
type DiffReport struct {
	OldVersion    int
	NewVersion    int
	Compatibility CompatibilityLevel
	Changes       []SchemaChange
}

// CompareDefinitions analyzes the differences and compatibility between old and new definitions.
func CompareDefinitions(oldDef, newDef *ttypes.WorkflowDefinition) DiffReport {
	report := DiffReport{
		OldVersion:    oldDef.ID.Version,
		NewVersion:    newDef.ID.Version,
		Compatibility: LevelIdentical,
	}

	oldSteps := make(map[string]ttypes.StepDefinition)
	for _, s := range oldDef.Steps {
		oldSteps[s.ID] = s
	}

	newSteps := make(map[string]ttypes.StepDefinition)
	for _, s := range newDef.Steps {
		newSteps[s.ID] = s
	}

	hasBreaking := false
	hasCompatible := false

	// Check removed steps (Breaking)
	for id, oldStep := range oldSteps {
		if _, exists := newSteps[id]; !exists {
			hasBreaking = true
			report.Changes = append(report.Changes, SchemaChange{
				Kind:        ChangeStepRemoved,
				StepID:      id,
				Description: fmt.Sprintf("step %q was removed", id),
				IsBreaking:  true,
			})
		} else {
			// Check modified step attributes
			newStep := newSteps[id]
			if oldStep.Handler != newStep.Handler {
				hasBreaking = true
				report.Changes = append(report.Changes, SchemaChange{
					Kind:        ChangeHandlerChanged,
					StepID:      id,
					Description: fmt.Sprintf("handler changed from %s to %s", oldStep.Handler, newStep.Handler),
					IsBreaking:  true,
				})
			}
			if !depsEqual(oldStep.DependsOn, newStep.DependsOn) {
				hasBreaking = true
				report.Changes = append(report.Changes, SchemaChange{
					Kind:        ChangeDepsChanged,
					StepID:      id,
					Description: fmt.Sprintf("dependencies changed for step %q", id),
					IsBreaking:  true,
				})
			}
			if oldStep.Timeout != newStep.Timeout {
				hasCompatible = true
				report.Changes = append(report.Changes, SchemaChange{
					Kind:        ChangeTimeoutChanged,
					StepID:      id,
					Description: fmt.Sprintf("timeout adjusted for step %q", id),
					IsBreaking:  false,
				})
			}
		}
	}

	// Check added steps
	for id := range newSteps {
		if _, exists := oldSteps[id]; !exists {
			hasCompatible = true
			report.Changes = append(report.Changes, SchemaChange{
				Kind:        ChangeStepAdded,
				StepID:      id,
				Description: fmt.Sprintf("step %q was added", id),
				IsBreaking:  false,
			})
		}
	}

	if hasBreaking {
		report.Compatibility = LevelBreaking
	} else if hasCompatible {
		report.Compatibility = LevelBackwardsCompatible
	}

	return report
}

func depsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool)
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

// UpgradeStrategy determines how running workflows adapt to definition updates.
type UpgradeStrategy string

const (
	StrategyPinToVersion UpgradeStrategy = "PIN_TO_VERSION"
	StrategyDrainAndExit UpgradeStrategy = "DRAIN_AND_EXIT"
	StrategyAutoMigrate  UpgradeStrategy = "AUTO_MIGRATE"
)

// PolicyDecider recommends an upgrade strategy given a diff report and run state.
func DecideUpgradeStrategy(diff DiffReport, state ttypes.RunState) UpgradeStrategy {
	if diff.Compatibility == LevelIdentical || state != ttypes.StateRunning {
		return StrategyAutoMigrate
	}
	if diff.Compatibility == LevelBreaking {
		// Running workflows cannot safely adopt breaking schema modifications
		return StrategyPinToVersion
	}
	return StrategyDrainAndExit
}

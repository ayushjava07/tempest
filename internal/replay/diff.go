package replay

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MutationType classifies how state was altered during a workflow step.
type MutationType string

const (
	MutationRead   MutationType = "READ"
	MutationWrite  MutationType = "WRITE"
	MutationDelete MutationType = "DELETE"
)

// Mutation represents a discrete variable access or modification.
type Mutation struct {
	StepID    string       `json:"step_id"`
	Type      MutationType `json:"type"`
	Key       string       `json:"key"`
	OldValue  string       `json:"old_value,omitempty"`
	NewValue  string       `json:"new_value,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
}

// FieldDiff describes a change in a single payload or state attribute.
type FieldDiff struct {
	Key      string `json:"key"`
	Original string `json:"original,omitempty"`
	Replayed string `json:"replayed,omitempty"`
}

// StateDiff captures structural divergence between recorded and replayed step states.
type StateDiff struct {
	StepID   string      `json:"step_id"`
	Added    []FieldDiff `json:"added,omitempty"`
	Removed  []FieldDiff `json:"removed,omitempty"`
	Modified []FieldDiff `json:"modified,omitempty"`
}

// HasDivergence returns true if any fields differ between original and replayed states.
func (d StateDiff) HasDivergence() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Modified) > 0
}

// Summary returns a human-readable description of state differences.
func (d StateDiff) Summary() string {
	if !d.HasDivergence() {
		return fmt.Sprintf("step %s: deterministic match (no divergence)", d.StepID)
	}

	var parts []string
	if len(d.Added) > 0 {
		var keys []string
		for _, f := range d.Added {
			keys = append(keys, fmt.Sprintf("%s=%q", f.Key, f.Replayed))
		}
		parts = append(parts, fmt.Sprintf("added: [%s]", strings.Join(keys, ", ")))
	}
	if len(d.Removed) > 0 {
		var keys []string
		for _, f := range d.Removed {
			keys = append(keys, fmt.Sprintf("%s=%q", f.Key, f.Original))
		}
		parts = append(parts, fmt.Sprintf("removed: [%s]", strings.Join(keys, ", ")))
	}
	if len(d.Modified) > 0 {
		var keys []string
		for _, f := range d.Modified {
			keys = append(keys, fmt.Sprintf("%s: %q -> %q", f.Key, f.Original, f.Replayed))
		}
		parts = append(parts, fmt.Sprintf("modified: [%s]", strings.Join(keys, ", ")))
	}

	return fmt.Sprintf("step %s divergence: %s", d.StepID, strings.Join(parts, "; "))
}

// DiffState computes structural differences between recorded original state and replayed state.
func DiffState(stepID string, original, replayed map[string]string) StateDiff {
	diff := StateDiff{
		StepID:   stepID,
		Added:    make([]FieldDiff, 0),
		Removed:  make([]FieldDiff, 0),
		Modified: make([]FieldDiff, 0),
	}

	// Check original keys
	for k, origVal := range original {
		repVal, exists := replayed[k]
		if !exists {
			diff.Removed = append(diff.Removed, FieldDiff{
				Key:      k,
				Original: origVal,
			})
		} else if origVal != repVal {
			diff.Modified = append(diff.Modified, FieldDiff{
				Key:      k,
				Original: origVal,
				Replayed: repVal,
			})
		}
	}

	// Check replayed keys for additions
	for k, repVal := range replayed {
		if _, exists := original[k]; !exists {
			diff.Added = append(diff.Added, FieldDiff{
				Key:      k,
				Replayed: repVal,
			})
		}
	}

	// Deterministic ordering of diff fields
	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].Key < diff.Added[j].Key })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].Key < diff.Removed[j].Key })
	sort.Slice(diff.Modified, func(i, j int) bool { return diff.Modified[i].Key < diff.Modified[j].Key })

	return diff
}

// StepMutationTracker records fine-grained variable reads, writes, and deletes during execution.
type StepMutationTracker struct {
	mu        sync.Mutex
	mutations []Mutation
}

// NewStepMutationTracker creates an empty mutation tracker.
func NewStepMutationTracker() *StepMutationTracker {
	return &StepMutationTracker{
		mutations: make([]Mutation, 0),
	}
}

// TrackRead logs a variable read access.
func (t *StepMutationTracker) TrackRead(stepID, key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.mutations = append(t.mutations, Mutation{
		StepID:    stepID,
		Type:      MutationRead,
		Key:       key,
		Timestamp: time.Now().UTC(),
	})
}

// TrackWrite logs a variable write mutation with old and new values.
func (t *StepMutationTracker) TrackWrite(stepID, key, oldValue, newValue string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.mutations = append(t.mutations, Mutation{
		StepID:    stepID,
		Type:      MutationWrite,
		Key:       key,
		OldValue:  oldValue,
		NewValue:  newValue,
		Timestamp: time.Now().UTC(),
	})
}

// TrackDelete logs a variable removal.
func (t *StepMutationTracker) TrackDelete(stepID, key, oldValue string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.mutations = append(t.mutations, Mutation{
		StepID:    stepID,
		Type:      MutationDelete,
		Key:       key,
		OldValue:  oldValue,
		Timestamp: time.Now().UTC(),
	})
}

// MutationsForStep returns all mutations belonging to a specific step in order.
func (t *StepMutationTracker) MutationsForStep(stepID string) []Mutation {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []Mutation
	for _, m := range t.mutations {
		if m.StepID == stepID {
			result = append(result, m)
		}
	}
	return result
}

// AllMutations returns a snapshot copy of all tracked mutations.
func (t *StepMutationTracker) AllMutations() []Mutation {
	t.mu.Lock()
	defer t.mu.Unlock()

	cp := make([]Mutation, len(t.mutations))
	copy(cp, t.mutations)
	return cp
}

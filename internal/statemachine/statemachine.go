package statemachine

import (
	"errors"
	"fmt"
	"sync"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

var (
	ErrIllegalTransition = errors.New("illegal state transition")
	ErrTerminalState     = errors.New("cannot transition from terminal state")
)

// TransitionHook is invoked whenever a successful state transition occurs.
type TransitionHook func(from, to ttypes.RunState)

// StateMachine enforces legal state transitions for workflow runs and steps.
type StateMachine struct {
	mu           sync.RWMutex
	legal        map[ttypes.RunState]map[ttypes.RunState]bool
	hooks        []TransitionHook
}

// New creates a new StateMachine initialized with legal execution transitions.
func New() *StateMachine {
	sm := &StateMachine{
		legal: make(map[ttypes.RunState]map[ttypes.RunState]bool),
	}

	// Define legal transitions
	sm.allow(ttypes.StatePending, ttypes.StateQueued)
	sm.allow(ttypes.StatePending, ttypes.StateCancelled)

	sm.allow(ttypes.StateQueued, ttypes.StateRunning)
	sm.allow(ttypes.StateQueued, ttypes.StateCancelled)

	sm.allow(ttypes.StateRunning, ttypes.StateSucceeded)
	sm.allow(ttypes.StateRunning, ttypes.StateFailed)
	sm.allow(ttypes.StateRunning, ttypes.StateCancelled)
	sm.allow(ttypes.StateRunning, ttypes.StateTimedOut)

	return sm
}

func (sm *StateMachine) allow(from, to ttypes.RunState) {
	if sm.legal[from] == nil {
		sm.legal[from] = make(map[ttypes.RunState]bool)
	}
	sm.legal[from][to] = true
}

// RegisterHook adds a transition hook callback.
func (sm *StateMachine) RegisterHook(hook TransitionHook) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.hooks = append(sm.hooks, hook)
}

// CanTransition checks whether moving from 'from' to 'to' is legally allowed.
func (sm *StateMachine) CanTransition(from, to ttypes.RunState) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if from.IsTerminal() {
		return false
	}

	destMap, ok := sm.legal[from]
	if !ok {
		return false
	}
	return destMap[to]
}

// Transition validates and executes a transition from 'from' to 'to'.
func (sm *StateMachine) Transition(from, to ttypes.RunState) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if from.IsTerminal() {
		return fmt.Errorf("%w: state %s is terminal", ErrTerminalState, from)
	}

	if !sm.CanTransition(from, to) {
		return fmt.Errorf("%w: cannot move from %s to %s", ErrIllegalTransition, from, to)
	}

	for _, hook := range sm.hooks {
		hook(from, to)
	}

	return nil
}

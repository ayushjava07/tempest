package lane

import (
	"sync"
	"time"
)

// BackpressureState classifies queue saturation levels.
type BackpressureState string

const (
	StateNormal     BackpressureState = "NORMAL"
	StateThrottling BackpressureState = "THROTTLING"
	StateRejecting  BackpressureState = "REJECTING"
)

// BackpressureAction indicates recommended caller action.
type BackpressureAction struct {
	State        BackpressureState `json:"state"`
	Allowed      bool              `json:"allowed"`
	RetryAfter   time.Duration     `json:"retry_after"`
	CurrentDepth int               `json:"current_depth"`
	MaxCapacity  int               `json:"max_capacity"`
}

// BackpressureObserver is invoked on state transitions.
type BackpressureObserver func(laneID string, oldState, newState BackpressureState)

// BackpressureController monitors lane queue utilization and signals producers.
type BackpressureController struct {
	mu            sync.RWMutex
	lanes         map[string]*Lane
	lowWatermark  float64 // e.g. 0.70 (70% capacity -> throttling)
	highWatermark float64 // e.g. 0.90 (90% capacity -> rejecting)
	states        map[string]BackpressureState
	observers     []BackpressureObserver
}

// NewBackpressureController initializes a backpressure controller.
func NewBackpressureController(lanes []*Lane, lowPct, highPct float64) *BackpressureController {
	if lowPct <= 0 || lowPct >= 1.0 {
		lowPct = 0.70
	}
	if highPct <= lowPct || highPct > 1.0 {
		highPct = 0.90
	}

	laneMap := make(map[string]*Lane, len(lanes))
	states := make(map[string]BackpressureState, len(lanes))
	for _, l := range lanes {
		laneMap[l.ID()] = l
		states[l.ID()] = StateNormal
	}

	return &BackpressureController{
		lanes:         laneMap,
		lowWatermark:  lowPct,
		highWatermark: highPct,
		states:        states,
		observers:     make([]BackpressureObserver, 0),
	}
}

// AddObserver registers a listener for backpressure level transitions.
func (b *BackpressureController) AddObserver(obs BackpressureObserver) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.observers = append(b.observers, obs)
}

// Check evaluates the specified lane depth and returns current backpressure advisory.
func (b *BackpressureController) Check(laneID string) BackpressureAction {
	b.mu.Lock()
	defer b.mu.Unlock()

	lane, exists := b.lanes[laneID]
	if !exists {
		return BackpressureAction{
			State:   StateNormal,
			Allowed: true,
		}
	}

	depth := lane.Len()
	cap := lane.cfg.MaxCapacity
	if cap <= 0 {
		cap = 1000
	}

	ratio := float64(depth) / float64(cap)
	prevState := b.states[laneID]
	var nextState BackpressureState

	if ratio >= b.highWatermark {
		nextState = StateRejecting
	} else if ratio >= b.lowWatermark {
		nextState = StateThrottling
	} else {
		nextState = StateNormal
	}

	if nextState != prevState {
		b.states[laneID] = nextState
		for _, obs := range b.observers {
			obs(laneID, prevState, nextState)
		}
	}

	action := BackpressureAction{
		State:        nextState,
		CurrentDepth: depth,
		MaxCapacity:  cap,
	}

	switch nextState {
	case StateRejecting:
		action.Allowed = false
		action.RetryAfter = 500 * time.Millisecond
	case StateThrottling:
		action.Allowed = true
		action.RetryAfter = 50 * time.Millisecond
	default:
		action.Allowed = true
		action.RetryAfter = 0
	}

	return action
}

// State returns current backpressure state for a given lane.
func (b *BackpressureController) State(laneID string) BackpressureState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if s, ok := b.states[laneID]; ok {
		return s
	}
	return StateNormal
}

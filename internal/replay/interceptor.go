package replay

import (
	"fmt"
	"sync"
)

// DivergenceType classifies the nature of non-determinism during replay.
type DivergenceType string

const (
	DivergenceTypeStepMismatch       DivergenceType = "StepMismatch"
	DivergenceTypeBranchDivergence   DivergenceType = "BranchDivergence"
	DivergenceTypeSideEffectMismatch DivergenceType = "SideEffectMismatch"
	DivergenceTypePayloadMismatch    DivergenceType = "PayloadMismatch"
)

// BranchDivergence describes a detected non-deterministic deviation.
type BranchDivergence struct {
	Type     DivergenceType    `json:"type"`
	StepID   string            `json:"step_id"`
	Expected string            `json:"expected"`
	Actual   string            `json:"actual"`
	Seq      uint64            `json:"seq"`
	Context  map[string]string `json:"context,omitempty"`
}

func (d BranchDivergence) Error() string {
	return fmt.Sprintf("%s at seq %d (step %s): expected %q, got %q",
		d.Type, d.Seq, d.StepID, d.Expected, d.Actual)
}

// SideEffectInterceptor manages deterministic recording and playback of side effects.
type SideEffectInterceptor struct {
	mu           sync.Mutex
	isReplaying  bool
	recordedData map[string]string
	recordedKeys []string
	cursor       int
}

// NewSideEffectInterceptor creates an interceptor for either recording or replay mode.
func NewSideEffectInterceptor(replaying bool) *SideEffectInterceptor {
	return &SideEffectInterceptor{
		isReplaying:  replaying,
		recordedData: make(map[string]string),
		recordedKeys: make([]string, 0),
	}
}

// LoadRecordedSideEffects loads previously captured side-effect outputs for replay.
func (s *SideEffectInterceptor) LoadRecordedSideEffects(effects map[string]string, order []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.recordedData = make(map[string]string, len(effects))
	for k, v := range effects {
		s.recordedData[k] = v
	}
	s.recordedKeys = make([]string, len(order))
	copy(s.recordedKeys, order)
	s.cursor = 0
}

// Intercept executes fn in live mode and records its result, or returns the recorded result during replay.
func (s *SideEffectInterceptor) Intercept(id string, fn func() (string, error)) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isReplaying {
		val, exists := s.recordedData[id]
		if !exists {
			return "", fmt.Errorf("%w: side effect %q not found in replay log",
				ErrNonDeterministicBranch, id)
		}
		s.cursor++
		return val, nil
	}

	// Live execution: run the side effect and record the outcome
	val, err := fn()
	if err != nil {
		return "", err
	}
	s.recordedData[id] = val
	s.recordedKeys = append(s.recordedKeys, id)
	return val, nil
}

// SideEffects returns a copy of all recorded side-effect pairs and their execution order.
func (s *SideEffectInterceptor) SideEffects() (map[string]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make(map[string]string, len(s.recordedData))
	for k, v := range s.recordedData {
		cp[k] = v
	}
	order := make([]string, len(s.recordedKeys))
	copy(order, s.recordedKeys)
	return cp, order
}

// BranchDetector tracks branch evaluations and verifies them against expected replay paths.
type BranchDetector struct {
	mu          sync.Mutex
	replayer    *Replayer
	divergences []BranchDivergence
}

// NewBranchDetector creates a detector attached to a replayer.
func NewBranchDetector(replayer *Replayer) *BranchDetector {
	return &BranchDetector{
		replayer:    replayer,
		divergences: make([]BranchDivergence, 0),
	}
}

// VerifyBranch asserts that the executed branch matches the recorded next step.
func (b *BranchDetector) VerifyBranch(stepID string, branchName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	evt, ok := b.replayer.Peek()
	if !ok {
		div := BranchDivergence{
			Type:     DivergenceTypeBranchDivergence,
			StepID:   stepID,
			Expected: "<none>",
			Actual:   branchName,
			Seq:      uint64(b.replayer.Cursor()),
		}
		b.divergences = append(b.divergences, div)
		return fmt.Errorf("%w: %s", ErrNonDeterministicBranch, div.Error())
	}

	// If recorded event is for this step, verify branch match in payload
	if evt.StepID != stepID {
		div := BranchDivergence{
			Type:     DivergenceTypeStepMismatch,
			StepID:   stepID,
			Expected: evt.StepID,
			Actual:   stepID,
			Seq:      evt.Seq,
		}
		b.divergences = append(b.divergences, div)
		return fmt.Errorf("%w: %s", ErrNonDeterministicBranch, div.Error())
	}

	if expectedBranch, hasBranch := evt.Payload["branch"]; hasBranch && expectedBranch != branchName {
		div := BranchDivergence{
			Type:     DivergenceTypeBranchDivergence,
			StepID:   stepID,
			Expected: expectedBranch,
			Actual:   branchName,
			Seq:      evt.Seq,
			Context:  evt.Payload,
		}
		b.divergences = append(b.divergences, div)
		return fmt.Errorf("%w: %s", ErrNonDeterministicBranch, div.Error())
	}

	return nil
}

// Divergences returns all recorded divergence events.
func (b *BranchDetector) Divergences() []BranchDivergence {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := make([]BranchDivergence, len(b.divergences))
	copy(cp, b.divergences)
	return cp
}

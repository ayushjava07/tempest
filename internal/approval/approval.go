package approval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrAlreadyDecided     = errors.New("approval: request has already reached final decision")
	ErrUnauthorizedPerson = errors.New("approval: approver not in authorized group")
	ErrDuplicateVote      = errors.New("approval: approver has already voted on this request")
	ErrRequestNotFound    = errors.New("approval: approval request not found")
	ErrInvalidPolicy      = errors.New("approval: invalid policy configuration")
)

type PolicyType string

const (
	PolicyAny    PolicyType = "ANY"
	PolicyAll    PolicyType = "ALL"
	PolicyQuorum PolicyType = "QUORUM"
)

type Decision string

const (
	DecisionPending  Decision = "PENDING"
	DecisionApproved Decision = "APPROVED"
	DecisionRejected Decision = "REJECTED"
	DecisionExpired  Decision = "EXPIRED"
)

type Vote struct {
	ApproverID string
	Approved   bool
	Comment    string
	VotedAt    time.Time
}

// Request represents a pending approval gateway in a workflow execution.
type Request struct {
	mu          sync.RWMutex
	ID          string
	Namespace   string
	RunID       string
	StepID      string
	Policy      PolicyType
	Approvers   map[string]bool
	QuorumCount int
	Timeout     time.Duration
	EscalateTo  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Decision    Decision
	DecidedAt   *time.Time
	Votes       map[string]Vote
	Escalated   bool
}

// Manager coordinates approval requests and vote collections.
type Manager struct {
	mu       sync.RWMutex
	requests map[string]*Request
}

func NewManager() *Manager {
	return &Manager{
		requests: make(map[string]*Request),
	}
}

// CreateRequest registers a new pending approval gate.
func (m *Manager) CreateRequest(id, ns, runID, stepID string, policy PolicyType, approvers []string, quorumCount int, timeout time.Duration, escalateTo string) (*Request, error) {
	if len(approvers) == 0 {
		return nil, fmt.Errorf("%w: at least one approver required", ErrInvalidPolicy)
	}
	if policy == PolicyQuorum && (quorumCount <= 0 || quorumCount > len(approvers)) {
		return nil, fmt.Errorf("%w: quorumCount must be between 1 and %d", ErrInvalidPolicy, len(approvers))
	}

	apprMap := make(map[string]bool, len(approvers))
	for _, a := range approvers {
		apprMap[a] = true
	}

	now := time.Now().UTC()
	var expiresAt time.Time
	if timeout > 0 {
		expiresAt = now.Add(timeout)
	}

	req := &Request{
		ID:          id,
		Namespace:   ns,
		RunID:       runID,
		StepID:      stepID,
		Policy:      policy,
		Approvers:   apprMap,
		QuorumCount: quorumCount,
		Timeout:     timeout,
		EscalateTo:  escalateTo,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		Decision:    DecisionPending,
		Votes:       make(map[string]Vote),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[id] = req
	return req, nil
}

// CastVote records an approval or rejection from an authorized approver.
func (m *Manager) CastVote(ctx context.Context, requestID, approverID string, approved bool, comment string) (Decision, error) {
	m.mu.RLock()
	req, ok := m.requests[requestID]
	m.mu.RUnlock()

	if !ok {
		return DecisionPending, ErrRequestNotFound
	}

	req.mu.Lock()
	defer req.mu.Unlock()

	if req.Decision != DecisionPending {
		return req.Decision, ErrAlreadyDecided
	}

	if !req.Approvers[approverID] {
		return DecisionPending, ErrUnauthorizedPerson
	}

	if _, exists := req.Votes[approverID]; exists {
		return DecisionPending, ErrDuplicateVote
	}

	now := time.Now().UTC()
	if !req.ExpiresAt.IsZero() && now.After(req.ExpiresAt) {
		req.Decision = DecisionExpired
		req.DecidedAt = &now
		return DecisionExpired, nil
	}

	req.Votes[approverID] = Vote{
		ApproverID: approverID,
		Approved:   approved,
		Comment:    comment,
		VotedAt:    now,
	}

	// Evaluate policy outcome
	dec := req.evaluatePolicyLocked(now)
	if dec != DecisionPending {
		req.Decision = dec
		req.DecidedAt = &now
	}

	return req.Decision, nil
}

func (r *Request) evaluatePolicyLocked(now time.Time) Decision {
	approveCount := 0
	rejectCount := 0
	totalApprovers := len(r.Approvers)

	for _, v := range r.Votes {
		if v.Approved {
			approveCount++
		} else {
			rejectCount++
		}
	}

	switch r.Policy {
	case PolicyAny:
		if approveCount > 0 {
			return DecisionApproved
		}
		if rejectCount == totalApprovers {
			return DecisionRejected
		}

	case PolicyAll:
		if rejectCount > 0 {
			return DecisionRejected
		}
		if approveCount == totalApprovers {
			return DecisionApproved
		}

	case PolicyQuorum:
		if approveCount >= r.QuorumCount {
			return DecisionApproved
		}
		// If remaining unvoted approvers cannot reach quorum
		remaining := totalApprovers - len(r.Votes)
		if approveCount+remaining < r.QuorumCount {
			return DecisionRejected
		}
	}

	return DecisionPending
}

// CheckEscalation marks request as escalated if deadline passed and still pending.
func (m *Manager) CheckEscalation(requestID string, now time.Time) bool {
	m.mu.RLock()
	req, ok := m.requests[requestID]
	m.mu.RUnlock()

	if !ok {
		return false
	}

	req.mu.Lock()
	defer req.mu.Unlock()

	if req.Decision == DecisionPending && !req.Escalated && req.EscalateTo != "" {
		if !req.ExpiresAt.IsZero() && now.After(req.ExpiresAt) {
			req.Escalated = true
			req.Approvers[req.EscalateTo] = true
			return true
		}
	}
	return false
}

// GetStatus inspects current decision state.
func (m *Manager) GetStatus(requestID string) (Decision, map[string]Vote, error) {
	m.mu.RLock()
	req, ok := m.requests[requestID]
	m.mu.RUnlock()

	if !ok {
		return DecisionPending, nil, ErrRequestNotFound
	}

	req.mu.RLock()
	defer req.mu.RUnlock()

	cpVotes := make(map[string]Vote, len(req.Votes))
	for k, v := range req.Votes {
		cpVotes[k] = v
	}
	return req.Decision, cpVotes, nil
}

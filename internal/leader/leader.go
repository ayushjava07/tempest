package leader

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNotLeader       = errors.New("leader: current instance is not the elected leader")
	ErrElectionClosed  = errors.New("leader: election coordinator closed")
	ErrElectionAborted = errors.New("leader: election campaign aborted")
)

// State identifies the candidate role in the cluster.
type State int

const (
	StateFollower State = iota
	StateCandidate
	StateLeader
)

func (s State) String() string {
	switch s {
	case StateFollower:
		return "FOLLOWER"
	case StateCandidate:
		return "CANDIDATE"
	case StateLeader:
		return "LEADER"
	default:
		return "UNKNOWN"
	}
}

// Config specifies timing parameters for leader election.
type Config struct {
	LeaseDuration time.Duration
	RenewInterval time.Duration
	RetryInterval time.Duration
}

// DefaultConfig provides sensible cluster defaults.
func DefaultConfig() Config {
	return Config{
		LeaseDuration: 500 * time.Millisecond,
		RenewInterval: 150 * time.Millisecond,
		RetryInterval: 100 * time.Millisecond,
	}
}

// Record captures leadership metadata and fencing term.
type Record struct {
	LeaderID  string    `json:"leader_id"`
	Term      uint64    `json:"term"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Coordinator handles lease arbitration for a named resource.
type Coordinator interface {
	TryAcquireOrRenew(candidateID string, term uint64, ttl time.Duration) (Record, bool, error)
	GetLeader() (Record, error)
	Release(candidateID string, term uint64) error
}

// Candidate represents a cluster node competing for leadership.
type Candidate struct {
	id        string
	coord     Coordinator
	cfg       Config
	state     State
	term      uint64
	isLeader  atomic.Bool
	mu        sync.RWMutex
	stopCh    chan struct{}
	electedCh chan struct{}
	revokedCh chan struct{}
	onElected []func()
	onRevoked []func()
}

// NewCandidate initializes a candidate instance.
func NewCandidate(id string, coord Coordinator, cfg Config) *Candidate {
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 500 * time.Millisecond
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = 150 * time.Millisecond
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 100 * time.Millisecond
	}

	return &Candidate{
		id:        id,
		coord:     coord,
		cfg:       cfg,
		state:     StateFollower,
		stopCh:    make(chan struct{}),
		electedCh: make(chan struct{}, 1),
		revokedCh: make(chan struct{}, 1),
	}
}

// ID returns the unique candidate identifier.
func (c *Candidate) ID() string {
	return c.id
}

// State returns current election state.
func (c *Candidate) State() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Term returns the current fencing term.
func (c *Candidate) Term() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.term
}

// Campaign attempts a single acquisition of leadership.
func (c *Candidate) Campaign() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = StateCandidate
	rec, acquired, err := c.coord.TryAcquireOrRenew(c.id, c.term, c.cfg.LeaseDuration)
	if err != nil {
		c.state = StateFollower
		return false, err
	}

	if acquired {
		c.state = StateLeader
		c.term = rec.Term
		c.isLeader.Store(true)
		return true, nil
	}

	c.state = StateFollower
	c.isLeader.Store(false)
	return false, nil
}

// Start boots the continuous election and heartbeat renewal loop.
func (c *Candidate) Start(ctx context.Context) {
	go c.runLoop(ctx)
}

func (c *Candidate) runLoop(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.stepDown()
			return
		case <-ctx.Done():
			c.stepDown()
			return
		case <-ticker.C:
			c.mu.Lock()
			isLead := (c.state == StateLeader)
			c.mu.Unlock()

			if isLead {
				// Renew heartbeat
				rec, renewed, err := c.coord.TryAcquireOrRenew(c.id, c.Term(), c.cfg.LeaseDuration)
				if err != nil || !renewed {
					c.stepDown()
					ticker.Reset(c.cfg.RetryInterval)
				} else {
					c.mu.Lock()
					c.term = rec.Term
					c.mu.Unlock()
					ticker.Reset(c.cfg.RenewInterval)
				}
			} else {
				// Campaign for leadership
				won, _ := c.Campaign()
				if won {
					c.triggerElected()
					ticker.Reset(c.cfg.RenewInterval)
				} else {
					ticker.Reset(c.cfg.RetryInterval)
				}
			}
		}
	}
}

func (c *Candidate) stepDown() {
	c.mu.Lock()
	wasLeader := (c.state == StateLeader)
	c.state = StateFollower
	c.isLeader.Store(false)
	term := c.term
	c.mu.Unlock()

	if wasLeader {
		_ = c.coord.Release(c.id, term)
		c.triggerRevoked()
	}
}

func (c *Candidate) triggerElected() {
	select {
	case c.electedCh <- struct{}{}:
	default:
	}
	c.mu.RLock()
	callbacks := make([]func(), len(c.onElected))
	copy(callbacks, c.onElected)
	c.mu.RUnlock()
	for _, cb := range callbacks {
		cb()
	}
}

func (c *Candidate) triggerRevoked() {
	select {
	case c.revokedCh <- struct{}{}:
	default:
	}
	c.mu.RLock()
	callbacks := make([]func(), len(c.onRevoked))
	copy(callbacks, c.onRevoked)
	c.mu.RUnlock()
	for _, cb := range callbacks {
		cb()
	}
}

// Stop terminates candidate loop and releases lease.
func (c *Candidate) Stop() {
	select {
	case <-c.stopCh:
		return
	default:
		close(c.stopCh)
	}
}

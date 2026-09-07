package lane

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLaneFull       = errors.New("lane: queue capacity exceeded")
	ErrLaneNotFound   = errors.New("lane: lane ID not recognized")
	ErrQueueEmpty     = errors.New("lane: all lanes are empty")
	ErrInvalidQuantum = errors.New("lane: quantum must be positive")
)

// Task represents a schedulable execution unit assigned to a lane.
type Task struct {
	ID         string    `json:"id"`
	LaneID     string    `json:"lane_id"`
	Payload    []byte    `json:"payload"`
	Cost       int       `json:"cost"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	AgeBoosted bool      `json:"age_boosted"`
}

// LaneConfig defines parameters for an individual priority lane.
type LaneConfig struct {
	ID                  string        `json:"id"`
	Priority            int           `json:"priority"`
	Quantum             int           `json:"quantum"`
	MaxCapacity         int           `json:"max_capacity"`
	StarvationThreshold time.Duration `json:"starvation_threshold"`
}

// Lane maintains an FIFO task queue and deficit round-robin accounting.
type Lane struct {
	mu            sync.RWMutex
	cfg           LaneConfig
	tasks         []*Task
	deficit       int
	enqueuedCount uint64
	dequeuedCount uint64
}

// NewLane initializes a single priority lane.
func NewLane(cfg LaneConfig) (*Lane, error) {
	if cfg.Quantum <= 0 {
		return nil, ErrInvalidQuantum
	}
	if cfg.MaxCapacity <= 0 {
		cfg.MaxCapacity = 10000
	}
	if cfg.StarvationThreshold <= 0 {
		cfg.StarvationThreshold = 30 * time.Second
	}

	return &Lane{
		cfg:   cfg,
		tasks: make([]*Task, 0),
	}, nil
}

// ID returns the lane identifier.
func (l *Lane) ID() string {
	return l.cfg.ID
}

// Priority returns the base configured priority.
func (l *Lane) Priority() int {
	return l.cfg.Priority
}

// Quantum returns the scheduling credit allowance per round.
func (l *Lane) Quantum() int {
	return l.cfg.Quantum
}

// Enqueue inserts a task into the lane, rejecting if capacity is reached.
func (l *Lane) Enqueue(task *Task) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.tasks) >= l.cfg.MaxCapacity {
		return fmt.Errorf("%w: lane %s at max capacity (%d)", ErrLaneFull, l.cfg.ID, l.cfg.MaxCapacity)
	}

	if task.Cost <= 0 {
		task.Cost = 1
	}
	if task.EnqueuedAt.IsZero() {
		task.EnqueuedAt = time.Now()
	}
	task.LaneID = l.cfg.ID

	l.tasks = append(l.tasks, task)
	l.enqueuedCount++
	return nil
}

// Dequeue removes and returns the head task.
func (l *Lane) Dequeue() (*Task, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.tasks) == 0 {
		return nil, false
	}

	t := l.tasks[0]
	l.tasks = l.tasks[1:]
	l.dequeuedCount++
	return t, true
}

// Peek returns the head task without dequeuing.
func (l *Lane) Peek() (*Task, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.tasks) == 0 {
		return nil, false
	}
	return l.tasks[0], true
}

// Len returns the current queue length.
func (l *Lane) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.tasks)
}

// OldestWaitDuration returns how long the head task has been waiting.
func (l *Lane) OldestWaitDuration() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.tasks) == 0 {
		return 0
	}
	return time.Since(l.tasks[0].EnqueuedAt)
}

// IsStarving returns true if the oldest task wait exceeds starvation threshold.
func (l *Lane) IsStarving() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.tasks) == 0 {
		return false
	}
	return time.Since(l.tasks[0].EnqueuedAt) >= l.cfg.StarvationThreshold
}

// Stats returns lane queue metrics.
type LaneStats struct {
	ID            string        `json:"id"`
	CurrentDepth  int           `json:"current_depth"`
	EnqueuedTotal uint64        `json:"enqueued_total"`
	DequeuedTotal uint64        `json:"dequeued_total"`
	OldestWait    time.Duration `json:"oldest_wait"`
	Deficit       int           `json:"deficit"`
}

// Stats snapshots the current lane state.
func (l *Lane) Stats() LaneStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var oldestWait time.Duration
	if len(l.tasks) > 0 {
		oldestWait = time.Since(l.tasks[0].EnqueuedAt)
	}

	return LaneStats{
		ID:            l.cfg.ID,
		CurrentDepth:  len(l.tasks),
		EnqueuedTotal: l.enqueuedCount,
		DequeuedTotal: l.dequeuedCount,
		OldestWait:    oldestWait,
		Deficit:       l.deficit,
	}
}

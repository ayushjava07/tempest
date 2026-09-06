package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"sync"
	"time"
)

var (
	ErrCorruptCheckpoint  = errors.New("checkpoint: data integrity check failed (CRC32 mismatch)")
	ErrNonMonotonicSeq    = errors.New("checkpoint: sequence number must be strictly monotonic")
	ErrCheckpointNotFound = errors.New("checkpoint: no checkpoint found for run")
	ErrInvalidRunID       = errors.New("checkpoint: run ID cannot be empty")
)

// Checkpoint represents a durable snapshot of workflow run progress at a step boundary.
type Checkpoint struct {
	ID        string
	Namespace string
	RunID     string
	StepID    string
	Sequence  uint64
	State     []byte
	Checksum  uint32
	Metadata  map[string]string
	CreatedAt time.Time
}

// ComputeChecksum calculates the CRC32 IEEE checksum of the checkpoint state buffer.
func ComputeChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// Validate verifies that the stored Checksum matches the CRC32 of the State bytes.
func (c *Checkpoint) Validate() error {
	expected := ComputeChecksum(c.State)
	if c.Checksum != expected {
		return fmt.Errorf("%w: expected 0x%08x, got 0x%08x", ErrCorruptCheckpoint, expected, c.Checksum)
	}
	return nil
}

// Store persists and retrieves checkpoints.
type Store interface {
	Save(ctx context.Context, cp *Checkpoint) error
	GetLatest(ctx context.Context, namespace, runID string) (*Checkpoint, error)
	List(ctx context.Context, namespace, runID string) ([]*Checkpoint, error)
	Prune(ctx context.Context, namespace, runID string, keepLatest int) (int, error)
}

// MemoryStore provides a thread-safe in-memory Store implementation.
type MemoryStore struct {
	mu          sync.RWMutex
	checkpoints map[string][]*Checkpoint // key: namespace:runID
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		checkpoints: make(map[string][]*Checkpoint),
	}
}

func (s *MemoryStore) key(namespace, runID string) string {
	return fmt.Sprintf("%s:%s", namespace, runID)
}

func (s *MemoryStore) Save(ctx context.Context, cp *Checkpoint) error {
	if cp == nil || cp.RunID == "" {
		return ErrInvalidRunID
	}
	if err := cp.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	k := s.key(cp.Namespace, cp.RunID)
	history := s.checkpoints[k]
	if len(history) > 0 {
		last := history[len(history)-1]
		if cp.Sequence <= last.Sequence {
			return fmt.Errorf("%w: proposed %d <= previous %d", ErrNonMonotonicSeq, cp.Sequence, last.Sequence)
		}
	}

	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}

	// Store copy
	cpCopy := *cp
	cpCopy.State = append([]byte(nil), cp.State...)
	s.checkpoints[k] = append(s.checkpoints[k], &cpCopy)
	return nil
}

func (s *MemoryStore) GetLatest(ctx context.Context, namespace, runID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := s.key(namespace, runID)
	history := s.checkpoints[k]
	if len(history) == 0 {
		return nil, ErrCheckpointNotFound
	}

	latest := history[len(history)-1]
	out := *latest
	out.State = append([]byte(nil), latest.State...)
	return &out, nil
}

func (s *MemoryStore) List(ctx context.Context, namespace, runID string) ([]*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := s.key(namespace, runID)
	history := s.checkpoints[k]
	out := make([]*Checkpoint, len(history))
	for i, cp := range history {
		c := *cp
		c.State = append([]byte(nil), cp.State...)
		out[i] = &c
	}
	return out, nil
}

func (s *MemoryStore) Prune(ctx context.Context, namespace, runID string, keepLatest int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if keepLatest <= 0 {
		keepLatest = 1
	}

	k := s.key(namespace, runID)
	history := s.checkpoints[k]
	if len(history) <= keepLatest {
		return 0, nil
	}

	pruneCount := len(history) - keepLatest
	s.checkpoints[k] = history[pruneCount:]
	return pruneCount, nil
}

// Engine coordinates creating and restoring checkpoints for workflow executions.
type Engine struct {
	store Store
}

func NewEngine(store Store) *Engine {
	return &Engine{store: store}
}

// CheckpointRun records a new execution state snapshot.
func (e *Engine) CheckpointRun(ctx context.Context, namespace, runID, stepID string, seq uint64, state []byte, metadata map[string]string) (*Checkpoint, error) {
	cp := &Checkpoint{
		ID:        fmt.Sprintf("cp-%s-%d", runID, seq),
		Namespace: namespace,
		RunID:     runID,
		StepID:    stepID,
		Sequence:  seq,
		State:     state,
		Checksum:  ComputeChecksum(state),
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	}

	if err := e.store.Save(ctx, cp); err != nil {
		return nil, fmt.Errorf("store checkpoint: %w", err)
	}

	return cp, nil
}

// RestoreLatest retrieves and verifies the most recent valid checkpoint for a run.
func (e *Engine) RestoreLatest(ctx context.Context, namespace, runID string) (*Checkpoint, error) {
	cp, err := e.store.GetLatest(ctx, namespace, runID)
	if err != nil {
		return nil, err
	}

	if err := cp.Validate(); err != nil {
		return nil, fmt.Errorf("corrupted checkpoint restored: %w", err)
	}

	return cp, nil
}

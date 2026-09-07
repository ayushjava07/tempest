package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type State map[string]any

type Checkpoint struct {
	ID        string
	Name      string
	State     State
	Timestamp time.Time
}

type Store interface {
	Save(ctx context.Context, cp *Checkpoint) error
	Load(ctx context.Context, id string) (*Checkpoint, error)
	List(ctx context.Context) ([]*Checkpoint, error)
	Delete(ctx context.Context, id string) error
}

type MemoryStore struct {
	mu          sync.RWMutex
	checkpoints map[string]*Checkpoint
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		checkpoints: make(map[string]*Checkpoint),
	}
}

func (ms *MemoryStore) Save(ctx context.Context, cp *Checkpoint) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.checkpoints[cp.ID] = cp
	return nil
}

func (ms *MemoryStore) Load(ctx context.Context, id string) (*Checkpoint, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	cp, ok := ms.checkpoints[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return cp, nil
}

func (ms *MemoryStore) List(ctx context.Context) ([]*Checkpoint, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	result := make([]*Checkpoint, 0, len(ms.checkpoints))
	for _, cp := range ms.checkpoints {
		result = append(result, cp)
	}
	return result, nil
}

func (ms *MemoryStore) Delete(ctx context.Context, id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.checkpoints, id)
	return nil
}

type Manager struct {
	store Store
}

func NewManager(store Store) *Manager {
	return &Manager{store: store}
}

func (m *Manager) Create(ctx context.Context, name string, state State) (*Checkpoint, error) {
	cp := &Checkpoint{
		ID:        fmt.Sprintf("cp-%d", time.Now().UnixNano()),
		Name:      name,
		State:     state,
		Timestamp: time.Now(),
	}
	if err := m.store.Save(ctx, cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func (m *Manager) Restore(ctx context.Context, id string) (State, error) {
	cp, err := m.store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	return cp.State, nil
}

func (m *Manager) List(ctx context.Context) ([]*Checkpoint, error) {
	return m.store.List(ctx)
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	return m.store.Delete(ctx, id)
}

func (cp *Checkpoint) Serialize() ([]byte, error) {
	return json.Marshal(cp)
}

func Deserialize(data []byte) (*Checkpoint, error) {
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}
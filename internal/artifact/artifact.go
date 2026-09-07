package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"
)

type Artifact struct {
	ID          string
	Name        string
	ContentType string
	Size        int64
	Checksum    string
	Metadata    map[string]string
}

type Store interface {
	Save(ctx context.Context, artifact *Artifact, data io.Reader) error
	Load(ctx context.Context, id string) (*Artifact, io.ReadCloser, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Artifact, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	artifacts map[string]*Artifact
	data      map[string][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		artifacts: make(map[string]*Artifact),
		data:      make(map[string][]byte),
	}
}

func (ms *MemoryStore) Save(ctx context.Context, artifact *Artifact, data io.Reader) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	content, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(content)
	artifact.Checksum = hex.EncodeToString(hash[:])
	artifact.Size = int64(len(content))
	ms.artifacts[artifact.ID] = artifact
	ms.data[artifact.ID] = content
	return nil
}

func (ms *MemoryStore) Load(ctx context.Context, id string) (*Artifact, io.ReadCloser, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	artifact, ok := ms.artifacts[id]
	if !ok {
		return nil, nil, fmt.Errorf("not found: %s", id)
	}
	return artifact, io.NopCloser(bytes.NewReader(ms.data[id])), nil
}

func (ms *MemoryStore) Delete(ctx context.Context, id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.artifacts, id)
	delete(ms.data, id)
	return nil
}

func (ms *MemoryStore) List(ctx context.Context) ([]*Artifact, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	result := make([]*Artifact, 0, len(ms.artifacts))
	for _, a := range ms.artifacts {
		result = append(result, a)
	}
	return result, nil
}

type Manager struct {
	store Store
}

func NewManager(store Store) *Manager {
	return &Manager{store: store}
}

func (m *Manager) Store(ctx context.Context, artifact *Artifact, data io.Reader) error {
	if artifact.ID == "" {
		artifact.ID = generateID()
	}
	return m.store.Save(ctx, artifact, data)
}

func (m *Manager) Get(ctx context.Context, id string) (*Artifact, io.ReadCloser, error) {
	return m.store.Load(ctx, id)
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	return m.store.Delete(ctx, id)
}

func (m *Manager) List(ctx context.Context) ([]*Artifact, error) {
	return m.store.List(ctx)
}

func generateID() string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return fmt.Sprintf("art-%x", hash[:8])
}
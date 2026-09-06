package artifact

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	ErrArtifactNotFound = errors.New("artifact not found")
	ErrStreamClosed     = errors.New("log stream is already closed")
)

// Artifact represents an immutable stored run output file or payload.
type Artifact struct {
	ID        string
	RunID     string
	Name      string
	MimeType  string
	Size      int64
	CreatedAt time.Time
	Data      []byte
}

// Store defines an interface for managing workflow artifacts and step logs.
type Store struct {
	mu        sync.RWMutex
	artifacts map[string]*Artifact
	logChunks map[string][][]byte // runID:stepID -> chunks
}

// NewStore creates a new in-memory artifact store.
func NewStore() *Store {
	return &Store{
		artifacts: make(map[string]*Artifact),
		logChunks: make(map[string][][]byte),
	}
}

// Put writes an artifact into the store.
func (s *Store) Put(runID, name, mimeType string, r io.Reader) (*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read artifact data: %w", err)
	}

	id := fmt.Sprintf("art-%s-%s", runID, name)
	art := &Artifact{
		ID:        id,
		RunID:     runID,
		Name:      name,
		MimeType:  mimeType,
		Size:      int64(len(data)),
		CreatedAt: time.Now(),
		Data:      data,
	}

	s.artifacts[id] = art
	return art, nil
}

// Get retrieves an artifact by ID.
func (s *Store) Get(id string) (*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	art, exists := s.artifacts[id]
	if !exists {
		return nil, ErrArtifactNotFound
	}
	return art, nil
}

// AppendLogChunk appends a chunk of stdout/stderr log output for a step.
func (s *Store) AppendLogChunk(runID, stepID string, chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", runID, stepID)
	cp := make([]byte, len(chunk))
	copy(cp, chunk)
	s.logChunks[key] = append(s.logChunks[key], cp)
}

// GetLogs returns the concatenated log bytes for a step.
func (s *Store) GetLogs(runID, stepID string) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", runID, stepID)
	chunks, exists := s.logChunks[key]
	if !exists {
		return nil
	}

	var buf bytes.Buffer
	for _, chunk := range chunks {
		buf.Write(chunk)
	}
	return buf.Bytes()
}

package compactor

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrKeyNotFound = errors.New("compactor: key not found")
	ErrStoreClosed = errors.New("compactor: store closed")
)

// Entry represents a versioned key-value record or tombstone.
type Entry struct {
	Key         string
	Value       []byte
	Seq         uint64
	IsTombstone bool
	Timestamp   time.Time
}

// SnapshotTable is an immutable sequence of sorted entries.
type SnapshotTable struct {
	ID      string
	MinKey  string
	MaxKey  string
	Entries []Entry
}

// NewSnapshotTable constructs an immutable table from sorted entries.
func NewSnapshotTable(id string, entries []Entry) *SnapshotTable {
	if len(entries) == 0 {
		return &SnapshotTable{ID: id}
	}
	return &SnapshotTable{
		ID:      id,
		MinKey:  entries[0].Key,
		MaxKey:  entries[len(entries)-1].Key,
		Entries: entries,
	}
}

func (s *SnapshotTable) Get(key string) (*Entry, bool) {
	if len(s.Entries) == 0 || key < s.MinKey || key > s.MaxKey {
		return nil, false
	}

	idx := sort.Search(len(s.Entries), func(i int) bool {
		return s.Entries[i].Key >= key
	})

	if idx < len(s.Entries) && s.Entries[idx].Key == key {
		return &s.Entries[idx], true
	}
	return nil, false
}

// Store manages incremental state checkpoints, memtables, and compaction.
type Store struct {
	mu          sync.RWMutex
	seqCounter  atomic.Uint64
	flushThresh int // number of entries before auto-flush
	memTable    map[string]Entry
	snapshots   []*SnapshotTable
	closed      bool
}

// Config configures compaction parameters.
type Config struct {
	FlushThreshold int
}

// DefaultConfig provides sane defaults.
func DefaultConfig() Config {
	return Config{
		FlushThreshold: 50,
	}
}

// New creates a new Compactor Store.
func New(cfg Config) *Store {
	if cfg.FlushThreshold <= 0 {
		cfg.FlushThreshold = 50
	}
	return &Store{
		flushThresh: cfg.FlushThreshold,
		memTable:    make(map[string]Entry),
		snapshots:   make([]*SnapshotTable, 0),
	}
}

// Put records a key-value mutation.
func (s *Store) Put(key string, val []byte) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	seq := s.seqCounter.Add(1)
	s.memTable[key] = Entry{
		Key:         key,
		Value:       append([]byte(nil), val...),
		Seq:         seq,
		IsTombstone: false,
		Timestamp:   time.Now().UTC(),
	}

	if len(s.memTable) >= s.flushThresh {
		s.flushLocked()
	}

	return seq, nil
}

// Delete writes a deletion tombstone.
func (s *Store) Delete(key string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	seq := s.seqCounter.Add(1)
	s.memTable[key] = Entry{
		Key:         key,
		Value:       nil,
		Seq:         seq,
		IsTombstone: true,
		Timestamp:   time.Now().UTC(),
	}

	if len(s.memTable) >= s.flushThresh {
		s.flushLocked()
	}

	return seq, nil
}

// Flush explicitly flushes the memTable into an immutable SnapshotTable.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	s.flushLocked()
	return nil
}

func (s *Store) flushLocked() {
	if len(s.memTable) == 0 {
		return
	}

	entries := make([]Entry, 0, len(s.memTable))
	for _, e := range s.memTable {
		entries = append(entries, e)
	}

	// Sort entries by Key ascending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	tableID := time.Now().Format("20060102150405.000000")
	snap := NewSnapshotTable(tableID, entries)
	s.snapshots = append(s.snapshots, snap)
	s.memTable = make(map[string]Entry)
}

// Get looks up the latest live value for key.
func (s *Store) Get(key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, false, ErrStoreClosed
	}

	// 1. Check mutable memTable first
	if e, ok := s.memTable[key]; ok {
		if e.IsTombstone {
			return nil, false, nil // deleted
		}
		return append([]byte(nil), e.Value...), true, nil
	}

	// 2. Check snapshot tables in reverse chronological order (newest to oldest)
	for i := len(s.snapshots) - 1; i >= 0; i-- {
		snap := s.snapshots[i]
		if e, ok := snap.Get(key); ok {
			if e.IsTombstone {
				return nil, false, nil
			}
			return append([]byte(nil), e.Value...), true, nil
		}
	}

	return nil, false, nil
}

// Compact merges all snapshots, purges tombstones, and retains only latest versions.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Flush memtable first
	s.flushLocked()

	if len(s.snapshots) <= 1 {
		// Nothing to merge if 0 or 1 snapshot
		if len(s.snapshots) == 1 {
			// Purge tombstones in the single snapshot
			var live []Entry
			for _, e := range s.snapshots[0].Entries {
				if !e.IsTombstone {
					live = append(live, e)
				}
			}
			s.snapshots[0] = NewSnapshotTable("compacted", live)
		}
		return nil
	}

	// Multi-way merge
	latestMap := make(map[string]Entry)
	for _, snap := range s.snapshots {
		for _, e := range snap.Entries {
			existing, ok := latestMap[e.Key]
			if !ok || e.Seq > existing.Seq {
				latestMap[e.Key] = e
			}
		}
	}

	var liveEntries []Entry
	for _, e := range latestMap {
		if !e.IsTombstone {
			liveEntries = append(liveEntries, e)
		}
	}

	sort.Slice(liveEntries, func(i, j int) bool {
		return liveEntries[i].Key < liveEntries[j].Key
	})

	compactedTable := NewSnapshotTable("compacted", liveEntries)
	s.snapshots = []*SnapshotTable{compactedTable}

	return nil
}

// Stats returns high-level storage footprint metrics.
func (s *Store) Stats() (totalLiveKeys int, numSnapshots int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	// Check memTable
	for k, e := range s.memTable {
		if !e.IsTombstone {
			seen[k] = true
		} else {
			seen[k] = false
		}
	}
	// Check snapshots
	for i := len(s.snapshots) - 1; i >= 0; i-- {
		for _, e := range s.snapshots[i].Entries {
			if _, exists := seen[e.Key]; !exists {
				if !e.IsTombstone {
					seen[e.Key] = true
				} else {
					seen[e.Key] = false
				}
			}
		}
	}

	liveCount := 0
	for _, isLive := range seen {
		if isLive {
			liveCount++
		}
	}

	return liveCount, len(s.snapshots)
}

// Close closes the store.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

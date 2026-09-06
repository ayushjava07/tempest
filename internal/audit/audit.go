package audit

import (
	"context"
	"sync"
	"time"
)

type Entry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Principal  string            `json:"principal"`
	Namespace  string            `json:"namespace"`
	Action     string            `json:"action"`
	Resource   string            `json:"resource"`
	ResourceID string            `json:"resource_id"`
	Outcome    string            `json:"outcome"`
	Details    map[string]string `json:"details,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type Log struct {
	mu      sync.RWMutex
	entries []Entry
	maxSize int
}

func NewLog(maxSize int) *Log {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Log{maxSize: maxSize}
}

func (l *Log) Record(entry Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.Timestamp = time.Now()
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
}

func (l *Log) List(limit int) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}
	start := len(l.entries) - limit
	out := make([]Entry, limit)
	copy(out, l.entries[start:])
	return out
}

func (l *Log) Size() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

type contextKey struct{}

func WithLog(ctx context.Context, log *Log) context.Context {
	return context.WithValue(ctx, contextKey{}, log)
}

func LogFrom(ctx context.Context) (*Log, bool) {
	if ctx == nil {
		return nil, false
	}
	l, ok := ctx.Value(contextKey{}).(*Log)
	return l, ok
}

func RecordFrom(ctx context.Context, entry Entry) {
	if l, ok := LogFrom(ctx); ok {
		l.Record(entry)
	}
}

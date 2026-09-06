package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

type Source interface {
	Load() ([]byte, error)
	Watch(ch chan<- struct{}) error
}

type Config struct {
	Sources []Source
}

type Loader struct {
	config   Config
	data     atomic.Value
	mu       sync.RWMutex
	watchers []chan struct{}
	loaded   bool
}

func New(config Config) *Loader {
	return &Loader{
		config: config,
	}
}

func (l *Loader) Load() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, source := range l.config.Sources {
		data, err := source.Load()
		if err != nil {
			return fmt.Errorf("load source: %w", err)
		}
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("parse source: %w", err)
		}
		l.data.Store(parsed)
		l.loaded = true
	}
	return nil
}

func (l *Loader) Get() interface{} {
	return l.data.Load()
}

func (l *Loader) GetBytes() ([]byte, error) {
	v := l.data.Load()
	if v == nil {
		return nil, fmt.Errorf("not loaded")
	}
	return json.Marshal(v)
}

func (l *Loader) IsLoaded() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.loaded
}

func (l *Loader) Reload() error {
	return l.Load()
}

type FileSource struct {
	Path string
}

func (fs *FileSource) Load() ([]byte, error) {
	return os.ReadFile(fs.Path)
}

func (fs *FileSource) Watch(ch chan<- struct{}) error {
	return nil
}

type MemorySource struct {
	Data []byte
}

func (ms *MemorySource) Load() ([]byte, error) {
	return ms.Data, nil
}

func (ms *MemorySource) Watch(ch chan<- struct{}) error {
	return nil
}

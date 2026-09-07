package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	WorkDir     string
	Timeout     time.Duration
	MaxMemoryMB int64
	AllowedDirs []string
}

type Sandbox struct {
	config Config
	mu     sync.Mutex
	procs  map[int]*exec.Cmd
}

func New(config Config) *Sandbox {
	if config.WorkDir == "" {
		config.WorkDir, _ = os.MkdirTemp("", "sandbox-*")
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	return &Sandbox{
		config: config,
		procs:  make(map[int]*exec.Cmd),
	}
}

func (s *Sandbox) Run(ctx context.Context, cmd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = s.config.WorkDir
	if len(s.config.AllowedDirs) > 0 {
		c.Env = append(os.Environ(), "ALLOWED_DIRS="+filepath.Join(s.config.AllowedDirs...))
	}
	output, err := c.CombinedOutput()
	return string(output), err
}

func (s *Sandbox) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.procs {
		if c.Process != nil {
			_ = c.Process.Kill()
		}
	}
	if s.config.WorkDir != "" {
		return os.RemoveAll(s.config.WorkDir)
	}
	return nil
}

func (s *Sandbox) WorkDir() string {
	return s.config.WorkDir
}

type FileSystem struct {
	root string
}

func NewFileSystem(root string) *FileSystem {
	return &FileSystem{root: root}
}

func (fs *FileSystem) Read(path string) ([]byte, error) {
	full := filepath.Join(fs.root, path)
	if !fs.isAllowed(full) {
		return nil, fmt.Errorf("access denied: %s", path)
	}
	return os.ReadFile(full)
}

func (fs *FileSystem) Write(path string, data []byte) error {
	full := filepath.Join(fs.root, path)
	if !fs.isAllowed(full) {
		return fmt.Errorf("access denied: %s", path)
	}
	return os.WriteFile(full, data, 0644)
}

func (fs *FileSystem) List(path string) ([]string, error) {
	full := filepath.Join(fs.root, path)
	if !fs.isAllowed(full) {
		return nil, fmt.Errorf("access denied: %s", path)
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func (fs *FileSystem) isAllowed(path string) bool {
	abs, _ := filepath.Abs(path)
	rootAbs, _ := filepath.Abs(fs.root)
	return filepath.HasPrefix(abs, rootAbs)
}
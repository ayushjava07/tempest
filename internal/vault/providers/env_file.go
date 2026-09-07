package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EnvProvider retrieves secrets from environment variables.
type EnvProvider struct {
	mu     sync.RWMutex
	prefix string
	// localOverrides for process-level hermetic testing
	overrides map[string]string
}

// NewEnvProvider creates a secret provider backed by environment variables.
func NewEnvProvider(prefix string) *EnvProvider {
	return &EnvProvider{
		prefix:    prefix,
		overrides: make(map[string]string),
	}
}

func (e *EnvProvider) Name() string {
	return "environment"
}

func (e *EnvProvider) sanitizeKey(key string) string {
	clean := strings.ToUpper(key)
	clean = strings.ReplaceAll(clean, "-", "_")
	clean = strings.ReplaceAll(clean, ".", "_")
	if e.prefix != "" {
		return e.prefix + clean
	}
	return clean
}

func (e *EnvProvider) GetSecret(ctx context.Context, key string) (*Secret, error) {
	e.mu.RLock()
	val, ok := e.overrides[key]
	e.mu.RUnlock()

	envKey := e.sanitizeKey(key)
	if !ok {
		val, ok = os.LookupEnv(envKey)
	}

	if !ok || val == "" {
		return nil, fmt.Errorf("%w: env variable %s not set", ErrSecretNotFound, envKey)
	}

	return &Secret{
		Key:   key,
		Value: []byte(val),
		Metadata: SecretMetadata{
			Key:       key,
			Version:   1,
			CreatedAt: time.Now().UTC(),
		},
	}, nil
}

func (e *EnvProvider) PutSecret(ctx context.Context, key string, value []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.overrides[key] = string(value)
	return nil
}

func (e *EnvProvider) DeleteSecret(ctx context.Context, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.overrides, key)
	return nil
}

func (e *EnvProvider) ListSecrets(ctx context.Context) ([]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var keys []string
	for k := range e.overrides {
		keys = append(keys, k)
	}
	return keys, nil
}

// FileProvider reads secrets from mounted filesystem directories.
type FileProvider struct {
	mu      sync.RWMutex
	baseDir string
}

// NewFileProvider creates a provider targeting a secrets volume mount directory.
func NewFileProvider(baseDir string) (*FileProvider, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create secrets dir %s: %w", baseDir, err)
	}
	return &FileProvider{
		baseDir: baseDir,
	}, nil
}

func (f *FileProvider) Name() string {
	return "filesystem"
}

func (f *FileProvider) resolvePath(key string) (string, error) {
	// Defend against path traversal
	cleaned := filepath.Clean(key)
	if strings.Contains(cleaned, "..") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("%w: invalid secret key path %q", ErrInvalidSecretKey, key)
	}
	return filepath.Join(f.baseDir, cleaned), nil
}

func (f *FileProvider) GetSecret(ctx context.Context, key string) (*Secret, error) {
	path, err := f.resolvePath(key)
	if err != nil {
		return nil, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: file secret %s does not exist", ErrSecretNotFound, key)
		}
		return nil, err
	}

	info, _ := os.Stat(path)
	modTime := time.Now().UTC()
	if info != nil {
		modTime = info.ModTime().UTC()
	}

	return &Secret{
		Key:   key,
		Value: data,
		Metadata: SecretMetadata{
			Key:       key,
			Version:   1,
			CreatedAt: modTime,
		},
	}, nil
}

func (f *FileProvider) PutSecret(ctx context.Context, key string, value []byte) error {
	path, err := f.resolvePath(key)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write secret with strict permissions (0600)
	return os.WriteFile(path, value, 0600)
}

func (f *FileProvider) DeleteSecret(ctx context.Context, key string) error {
	path, err := f.resolvePath(key)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (f *FileProvider) ListSecrets(ctx context.Context) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

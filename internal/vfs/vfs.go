package vfs

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

var (
	ErrPathEscapesRoot      = errors.New("vfs: path traversal escapes tenant root")
	ErrFileNotFound         = errors.New("vfs: file not found")
	ErrStorageQuotaExceeded = errors.New("vfs: tenant storage quota exceeded")
	ErrFileCountExceeded    = errors.New("vfs: tenant file count limit exceeded")
)

// FileInfo provides metadata on a virtual file.
type FileInfo struct {
	Path      string
	SizeBytes int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type virtualFile struct {
	info FileInfo
	data []byte
}

// Config defines storage quotas for the virtual filesystem.
type Config struct {
	MaxTotalBytes int64
	MaxFileCount  int
}

func DefaultConfig() Config {
	return Config{
		MaxTotalBytes: 50 * 1024 * 1024, // 50 MB
		MaxFileCount:  500,
	}
}

// VFS implements a chrooted in-memory virtual filesystem.
type VFS struct {
	mu         sync.RWMutex
	root       string
	cfg        Config
	files      map[string]*virtualFile
	totalBytes int64
}

func New(root string, cfg Config) *VFS {
	if root == "" {
		root = "/"
	}
	cleanRoot := path.Clean("/" + root)
	return &VFS{
		root:  cleanRoot,
		cfg:   cfg,
		files: make(map[string]*virtualFile),
	}
}

// resolvePath canonicalizes user input and ensures it stays strictly within the chroot jail.
func (v *VFS) resolvePath(userPath string) (string, error) {
	if strings.Contains(userPath, "\x00") {
		return "", errors.New("vfs: invalid null byte in path")
	}

	cleanedRel := path.Clean(userPath)
	if strings.HasPrefix(userPath, "/..") || strings.Contains(userPath, "/../") || strings.HasPrefix(cleanedRel, "..") {
		return "", fmt.Errorf("%w: %s", ErrPathEscapesRoot, userPath)
	}

	clean := path.Clean("/" + userPath)
	full := path.Clean(path.Join(v.root, clean))

	rootWithSlash := v.root
	if !strings.HasSuffix(rootWithSlash, "/") {
		rootWithSlash += "/"
	}
	if full != v.root && !strings.HasPrefix(full, rootWithSlash) {
		return "", fmt.Errorf("%w: %s", ErrPathEscapesRoot, userPath)
	}

	return full, nil
}

// WriteFile writes data to virtual path, enforcing quota constraints.
func (v *VFS) WriteFile(userPath string, data []byte) error {
	resolved, err := v.resolvePath(userPath)
	if err != nil {
		return err
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	dataLen := int64(len(data))
	existing, ok := v.files[resolved]

	if !ok {
		// New file: check file count limit
		if v.cfg.MaxFileCount > 0 && len(v.files) >= v.cfg.MaxFileCount {
			return ErrFileCountExceeded
		}
		// Check byte quota
		if v.cfg.MaxTotalBytes > 0 && v.totalBytes+dataLen > v.cfg.MaxTotalBytes {
			return ErrStorageQuotaExceeded
		}
		v.totalBytes += dataLen
		now := time.Now().UTC()
		v.files[resolved] = &virtualFile{
			info: FileInfo{
				Path:      resolved,
				SizeBytes: dataLen,
				CreatedAt: now,
				UpdatedAt: now,
			},
			data: append([]byte(nil), data...),
		}
		return nil
	}

	// Overwrite existing: adjust byte quota difference
	diff := dataLen - existing.info.SizeBytes
	if v.cfg.MaxTotalBytes > 0 && v.totalBytes+diff > v.cfg.MaxTotalBytes {
		return ErrStorageQuotaExceeded
	}

	v.totalBytes += diff
	existing.info.SizeBytes = dataLen
	existing.info.UpdatedAt = time.Now().UTC()
	existing.data = append([]byte(nil), data...)
	return nil
}

// ReadFile retrieves the contents of the virtual file.
func (v *VFS) ReadFile(userPath string) ([]byte, error) {
	resolved, err := v.resolvePath(userPath)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	f, ok := v.files[resolved]
	if !ok {
		return nil, ErrFileNotFound
	}

	return append([]byte(nil), f.data...), nil
}

// RemoveFile deletes a file and updates the tenant quota.
func (v *VFS) RemoveFile(userPath string) error {
	resolved, err := v.resolvePath(userPath)
	if err != nil {
		return err
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	f, ok := v.files[resolved]
	if !ok {
		return ErrFileNotFound
	}

	v.totalBytes -= f.info.SizeBytes
	delete(v.files, resolved)
	return nil
}

// Stat inspects metadata of a file.
func (v *VFS) Stat(userPath string) (FileInfo, error) {
	resolved, err := v.resolvePath(userPath)
	if err != nil {
		return FileInfo{}, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	f, ok := v.files[resolved]
	if !ok {
		return FileInfo{}, ErrFileNotFound
	}

	return f.info, nil
}

// ListFiles lists all files under a prefix path.
func (v *VFS) ListFiles(prefix string) []FileInfo {
	resolved, _ := v.resolvePath(prefix)

	v.mu.RLock()
	defer v.mu.RUnlock()

	var out []FileInfo
	for p, f := range v.files {
		if strings.HasPrefix(p, resolved) {
			out = append(out, f.info)
		}
	}
	return out
}

// TotalUsage returns the currently consumed bytes and file count.
func (v *VFS) TotalUsage() (int64, int) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.totalBytes, len(v.files)
}

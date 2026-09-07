package blobstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrBlobNotFound      = errors.New("blobstore: blob not found")
	ErrInvalidKey        = errors.New("blobstore: invalid or empty key")
	ErrInvalidRange      = errors.New("blobstore: invalid byte range")
	ErrUploadNotFound    = errors.New("blobstore: multipart upload not found")
	ErrChecksumMismatch  = errors.New("blobstore: checksum mismatch")
	ErrPartNumberInvalid = errors.New("blobstore: part number must be > 0")
)

// BlobMeta contains metadata and content verification digests.
type BlobMeta struct {
	Key            string            `json:"key"`
	Size           int64             `json:"size"`
	SHA256         string            `json:"sha256"`
	ETag           string            `json:"etag"`
	ContentType    string            `json:"content_type,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	CustomMetadata map[string]string `json:"custom_metadata,omitempty"`
}

// Backend defines the common interface for blob storage providers.
type Backend interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, meta map[string]string) (*BlobMeta, error)
	Get(ctx context.Context, key string) (io.ReadCloser, *BlobMeta, error)
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	List(ctx context.Context, prefix string) ([]*BlobMeta, error)
}

// MemoryBackend is an in-memory thread-safe blob store.
type MemoryBackend struct {
	mu    sync.RWMutex
	blobs map[string]*memBlob
}

type memBlob struct {
	meta *BlobMeta
	data []byte
}

// NewMemoryBackend creates a new in-memory blob backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		blobs: make(map[string]*memBlob),
	}
}

func (m *MemoryBackend) Put(ctx context.Context, key string, r io.Reader, size int64, meta map[string]string) (*BlobMeta, error) {
	if key == "" {
		return nil, ErrInvalidKey
	}

	h := sha256.New()
	var buf bytes.Buffer
	tr := io.TeeReader(r, h)

	written, err := io.Copy(&buf, tr)
	if err != nil {
		return nil, err
	}
	if size > 0 && written != size {
		return nil, fmt.Errorf("expected size %d, wrote %d", size, written)
	}

	digest := hex.EncodeToString(h.Sum(nil))
	blobMeta := &BlobMeta{
		Key:            key,
		Size:           written,
		SHA256:         digest,
		ETag:           fmt.Sprintf(`"%s"`, digest[:16]),
		CreatedAt:      time.Now().UTC(),
		CustomMetadata: meta,
	}

	m.mu.Lock()
	m.blobs[key] = &memBlob{
		meta: blobMeta,
		data: buf.Bytes(),
	}
	m.mu.Unlock()

	return blobMeta, nil
}

func (m *MemoryBackend) Get(ctx context.Context, key string) (io.ReadCloser, *BlobMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, ok := m.blobs[key]
	if !ok {
		return nil, nil, ErrBlobNotFound
	}

	cp := make([]byte, len(b.data))
	copy(cp, b.data)
	return io.NopCloser(bytes.NewReader(cp)), b.meta, nil
}

func (m *MemoryBackend) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, ok := m.blobs[key]
	if !ok {
		return nil, ErrBlobNotFound
	}

	total := int64(len(b.data))
	if offset < 0 || offset > total || length < 0 || offset+length > total {
		return nil, ErrInvalidRange
	}

	sub := make([]byte, length)
	copy(sub, b.data[offset:offset+length])
	return io.NopCloser(bytes.NewReader(sub)), nil
}

func (m *MemoryBackend) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.blobs[key]; !ok {
		return ErrBlobNotFound
	}
	delete(m.blobs, key)
	return nil
}

func (m *MemoryBackend) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.blobs[key]
	return ok, nil
}

func (m *MemoryBackend) List(ctx context.Context, prefix string) ([]*BlobMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*BlobMeta
	for k, b := range m.blobs {
		if strings.HasPrefix(k, prefix) {
			results = append(results, b.meta)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Key < results[j].Key
	})
	return results, nil
}

// FSBackend implements local filesystem directory storage.
type FSBackend struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFSBackend initializes local disk storage under baseDir.
func NewFSBackend(baseDir string) (*FSBackend, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &FSBackend{baseDir: baseDir}, nil
}

func (f *FSBackend) keyPath(key string) string {
	// Sanitize key and escape
	clean := filepath.Clean(key)
	clean = strings.TrimPrefix(clean, "/")
	return filepath.Join(f.baseDir, clean)
}

func (f *FSBackend) Put(ctx context.Context, key string, r io.Reader, size int64, meta map[string]string) (*BlobMeta, error) {
	if key == "" || strings.Contains(key, "..") {
		return nil, ErrInvalidKey
	}

	target := f.keyPath(key)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	tmpFile, err := os.CreateTemp(dir, "tempest-blob-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	h := sha256.New()
	tr := io.TeeReader(r, h)

	written, err := io.Copy(tmpFile, tr)
	_ = tmpFile.Close()
	if err != nil {
		return nil, err
	}
	if size > 0 && written != size {
		return nil, fmt.Errorf("expected size %d, wrote %d", size, written)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if err := os.Rename(tmpName, target); err != nil {
		return nil, err
	}

	digest := hex.EncodeToString(h.Sum(nil))
	return &BlobMeta{
		Key:            key,
		Size:           written,
		SHA256:         digest,
		ETag:           fmt.Sprintf(`"%s"`, digest[:16]),
		CreatedAt:      time.Now().UTC(),
		CustomMetadata: meta,
	}, nil
}

func (f *FSBackend) Get(ctx context.Context, key string) (io.ReadCloser, *BlobMeta, error) {
	if key == "" || strings.Contains(key, "..") {
		return nil, nil, ErrInvalidKey
	}

	target := f.keyPath(key)
	f.mu.RLock()
	defer f.mu.RUnlock()

	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrBlobNotFound
		}
		return nil, nil, err
	}

	file, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}

	meta := &BlobMeta{
		Key:       key,
		Size:      info.Size(),
		CreatedAt: info.ModTime().UTC(),
	}

	return file, meta, nil
}

func (f *FSBackend) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if key == "" || strings.Contains(key, "..") {
		return nil, ErrInvalidKey
	}

	target := f.keyPath(key)
	f.mu.RLock()
	defer f.mu.RUnlock()

	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlobNotFound
		}
		return nil, err
	}

	if offset < 0 || offset > info.Size() || length < 0 || offset+length > info.Size() {
		return nil, ErrInvalidRange
	}

	file, err := os.Open(target)
	if err != nil {
		return nil, err
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, err
	}

	return &rangeReadCloser{
		Reader: io.LimitReader(file, length),
		Closer: file,
	}, nil
}

type rangeReadCloser struct {
	io.Reader
	io.Closer
}

func (f *FSBackend) Delete(ctx context.Context, key string) error {
	if key == "" || strings.Contains(key, "..") {
		return ErrInvalidKey
	}

	target := f.keyPath(key)
	f.mu.Lock()
	defer f.mu.Unlock()

	err := os.Remove(target)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrBlobNotFound
		}
		return err
	}
	return nil
}

func (f *FSBackend) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" || strings.Contains(key, "..") {
		return false, ErrInvalidKey
	}

	target := f.keyPath(key)
	f.mu.RLock()
	defer f.mu.RUnlock()

	_, err := os.Stat(target)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (f *FSBackend) List(ctx context.Context, prefix string) ([]*BlobMeta, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var results []*BlobMeta
	err := filepath.Walk(f.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(f.baseDir, path)
		if err != nil {
			return nil
		}

		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, prefix) {
			results = append(results, &BlobMeta{
				Key:       rel,
				Size:      info.Size(),
				CreatedAt: info.ModTime().UTC(),
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Key < results[j].Key
	})
	return results, nil
}

// PartInfo represents an uploaded multipart chunk.
type PartInfo struct {
	PartNumber int    `json:"part_number"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	ETag       string `json:"etag"`
}

type uploadState struct {
	key       string
	parts     map[int]*partData
	createdAt time.Time
}

type partData struct {
	info PartInfo
	data []byte
}

// MultipartCoordinator handles multi-chunk object uploads.
type MultipartCoordinator struct {
	mu      sync.Mutex
	backend Backend
	uploads map[string]*uploadState
}

// NewMultipartCoordinator wraps a Backend with multipart capabilities.
func NewMultipartCoordinator(b Backend) *MultipartCoordinator {
	return &MultipartCoordinator{
		backend: b,
		uploads: make(map[string]*uploadState),
	}
}

// CreateMultipart initializes a multi-part upload session.
func (c *MultipartCoordinator) CreateMultipart(key string) (string, error) {
	if key == "" {
		return "", ErrInvalidKey
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	uploadID := hex.EncodeToString(b)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.uploads[uploadID] = &uploadState{
		key:       key,
		parts:     make(map[int]*partData),
		createdAt: time.Now(),
	}

	return uploadID, nil
}

// UploadPart stores a numbered part for an active multipart upload.
func (c *MultipartCoordinator) UploadPart(uploadID string, partNumber int, r io.Reader) (PartInfo, error) {
	if partNumber <= 0 {
		return PartInfo{}, ErrPartNumberInvalid
	}

	c.mu.Lock()
	state, ok := c.uploads[uploadID]
	c.mu.Unlock()

	if !ok {
		return PartInfo{}, ErrUploadNotFound
	}

	h := sha256.New()
	var buf bytes.Buffer
	tr := io.TeeReader(r, h)

	written, err := io.Copy(&buf, tr)
	if err != nil {
		return PartInfo{}, err
	}

	digest := hex.EncodeToString(h.Sum(nil))
	info := PartInfo{
		PartNumber: partNumber,
		Size:       written,
		SHA256:     digest,
		ETag:       fmt.Sprintf(`"%s"`, digest[:16]),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state.parts[partNumber] = &partData{
		info: info,
		data: buf.Bytes(),
	}

	return info, nil
}

// CompleteMultipart stitches together all uploaded parts and commits the final blob.
func (c *MultipartCoordinator) CompleteMultipart(ctx context.Context, uploadID string, parts []PartInfo) (*BlobMeta, error) {
	c.mu.Lock()
	state, ok := c.uploads[uploadID]
	if !ok {
		c.mu.Unlock()
		return nil, ErrUploadNotFound
	}
	delete(c.uploads, uploadID)
	c.mu.Unlock()

	// Sort parts by PartNumber
	sortedParts := make([]PartInfo, len(parts))
	copy(sortedParts, parts)
	sort.Slice(sortedParts, func(i, j int) bool {
		return sortedParts[i].PartNumber < sortedParts[j].PartNumber
	})

	var combined bytes.Buffer
	for _, p := range sortedParts {
		part, ok := state.parts[p.PartNumber]
		if !ok {
			return nil, fmt.Errorf("%w: missing part %d", ErrUploadNotFound, p.PartNumber)
		}
		if p.SHA256 != "" && p.SHA256 != part.info.SHA256 {
			return nil, ErrChecksumMismatch
		}
		combined.Write(part.data)
	}

	meta, err := c.backend.Put(ctx, state.key, &combined, int64(combined.Len()), nil)
	if err != nil {
		return nil, err
	}

	return meta, nil
}

// AbortMultipart cancels and discards an in-flight upload session.
func (c *MultipartCoordinator) AbortMultipart(uploadID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.uploads[uploadID]; !ok {
		return ErrUploadNotFound
	}

	delete(c.uploads, uploadID)
	return nil
}

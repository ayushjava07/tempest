package fastjson

import (
	"sync"
)

const (
	defaultInitialBufferSize = 512
	maxRecycledBufferSize    = 64 * 1024 // Discard buffers > 64KB
)

// Buffer wraps a byte slice with high-efficiency serialization helpers.
type Buffer struct {
	buf []byte
}

// NewBuffer creates a buffer with pre-allocated capacity.
func NewBuffer(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = defaultInitialBufferSize
	}
	return &Buffer{
		buf: make([]byte, 0, capacity),
	}
}

// AppendByte appends a single byte.
func (b *Buffer) AppendByte(c byte) {
	b.buf = append(b.buf, c)
}

// AppendBytes appends a slice of bytes.
func (b *Buffer) AppendBytes(p []byte) {
	b.buf = append(b.buf, p...)
}

// AppendString appends a string.
func (b *Buffer) AppendString(s string) {
	b.buf = append(b.buf, s...)
}

// Bytes returns the written byte slice.
func (b *Buffer) Bytes() []byte {
	return b.buf
}

// String returns the buffer contents as a string.
func (b *Buffer) String() string {
	return string(b.buf)
}

// Len returns current length.
func (b *Buffer) Len() int {
	return len(b.buf)
}

// Cap returns current capacity.
func (b *Buffer) Cap() int {
	return cap(b.buf)
}

// Reset clears the buffer without freeing capacity.
func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
}

// BufferPool maintains a pool of recycled Buffer pointers.
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates an isolated buffer pool.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				return NewBuffer(defaultInitialBufferSize)
			},
		},
	}
}

// Get acquires a buffer from the pool.
func (p *BufferPool) Get() *Buffer {
	b := p.pool.Get().(*Buffer)
	b.Reset()
	return b
}

// Put returns a buffer to the pool if within the maximum capacity bound.
func (p *BufferPool) Put(b *Buffer) {
	if b == nil || cap(b.buf) > maxRecycledBufferSize {
		return
	}
	b.Reset()
	p.pool.Put(b)
}

// Global default pool
var globalPool = NewBufferPool()

// Acquire returns a recycled buffer from the global pool.
func Acquire() *Buffer {
	return globalPool.Get()
}

// Release returns a buffer to the global pool.
func Release(b *Buffer) {
	globalPool.Put(b)
}

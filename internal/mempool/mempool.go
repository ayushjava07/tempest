package mempool

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	ErrMemoryExhausted    = errors.New("mempool: global memory limit exhausted")
	ErrBufferTooLarge     = errors.New("mempool: requested size exceeds maximum slab class")
	ErrBufferAlreadyFreed = errors.New("mempool: buffer already released back to pool")
)

// Slab sizes in bytes
const (
	SlabSmall  = 4 * 1024        // 4 KB
	SlabMedium = 64 * 1024       // 64 KB
	SlabLarge  = 512 * 1024      // 512 KB
	SlabJumbo  = 4 * 1024 * 1024 // 4 MB
)

// Buffer wraps a pooled byte slice with safety tracking.
type Buffer struct {
	pool      *Pool
	slabClass int
	Data      []byte
	freed     atomic.Bool
}

// Release returns the buffer to its parent slab pool.
func (b *Buffer) Release() error {
	if b.freed.CompareAndSwap(false, true) {
		b.pool.returnBuffer(b)
		return nil
	}
	return ErrBufferAlreadyFreed
}

// Pool coordinates slab allocators with a strict global memory ceiling.
type Pool struct {
	mu           sync.Mutex
	maxBytes     int64
	currentBytes atomic.Int64

	smallPool  sync.Pool
	mediumPool sync.Pool
	largePool  sync.Pool
	jumboPool  sync.Pool

	allocs atomic.Uint64
	frees  atomic.Uint64
}

func NewPool(maxBytes int64) *Pool {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024 * 1024 // 64 MB default
	}

	p := &Pool{
		maxBytes: maxBytes,
	}

	p.smallPool.New = func() any { return make([]byte, SlabSmall) }
	p.mediumPool.New = func() any { return make([]byte, SlabMedium) }
	p.largePool.New = func() any { return make([]byte, SlabLarge) }
	p.jumboPool.New = func() any { return make([]byte, SlabJumbo) }

	return p
}

// Acquire allocates a buffer capable of holding at least size bytes.
func (p *Pool) Acquire(size int) (*Buffer, error) {
	slabSize := p.selectSlab(size)
	if slabSize == 0 {
		return nil, fmt.Errorf("%w: %d > %d", ErrBufferTooLarge, size, SlabJumbo)
	}

	// Check global memory budget
	newUsage := p.currentBytes.Add(int64(slabSize))
	if newUsage > p.maxBytes {
		p.currentBytes.Add(-int64(slabSize))
		return nil, ErrMemoryExhausted
	}

	var raw []byte
	switch slabSize {
	case SlabSmall:
		raw = p.smallPool.Get().([]byte)
	case SlabMedium:
		raw = p.mediumPool.Get().([]byte)
	case SlabLarge:
		raw = p.largePool.Get().([]byte)
	case SlabJumbo:
		raw = p.jumboPool.Get().([]byte)
	}

	p.allocs.Add(1)

	return &Buffer{
		pool:      p,
		slabClass: slabSize,
		Data:      raw[:size],
	}, nil
}

func (p *Pool) returnBuffer(b *Buffer) {
	// Zero memory to prevent cross-tenant data leakage
	clearBytes(b.Data)

	p.currentBytes.Add(-int64(b.slabClass))
	p.frees.Add(1)

	switch b.slabClass {
	case SlabSmall:
		p.smallPool.Put(b.Data[:SlabSmall])
	case SlabMedium:
		p.mediumPool.Put(b.Data[:SlabMedium])
	case SlabLarge:
		p.largePool.Put(b.Data[:SlabLarge])
	case SlabJumbo:
		p.jumboPool.Put(b.Data[:SlabJumbo])
	}
}

func (p *Pool) selectSlab(size int) int {
	if size <= SlabSmall {
		return SlabSmall
	}
	if size <= SlabMedium {
		return SlabMedium
	}
	if size <= SlabLarge {
		return SlabLarge
	}
	if size <= SlabJumbo {
		return SlabJumbo
	}
	return 0
}

func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// CurrentUsage returns current allocated bytes and maximum limit.
func (p *Pool) CurrentUsage() (int64, int64) {
	return p.currentBytes.Load(), p.maxBytes
}

// Stats returns allocation and free counters.
func (p *Pool) Stats() (uint64, uint64) {
	return p.allocs.Load(), p.frees.Load()
}

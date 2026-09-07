package mempool

import (
	"errors"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMempool_AcquireAndRelease(t *testing.T) {
	pool := NewPool(10 * 1024 * 1024)

	// Small slab
	buf, err := pool.Acquire(1024)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if len(buf.Data) != 1024 || buf.slabClass != SlabSmall {
		t.Errorf("expected 1024 bytes in SlabSmall, got %d in %d", len(buf.Data), buf.slabClass)
	}

	used, _ := pool.CurrentUsage()
	if used != SlabSmall {
		t.Errorf("expected %d bytes used, got %d", SlabSmall, used)
	}

	if err := buf.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	usedAfter, _ := pool.CurrentUsage()
	if usedAfter != 0 {
		t.Errorf("expected 0 bytes used after release, got %d", usedAfter)
	}

	// Double release must error
	if err := buf.Release(); !errors.Is(err, ErrBufferAlreadyFreed) {
		t.Errorf("expected ErrBufferAlreadyFreed on double release, got %v", err)
	}
}

func TestMempool_GlobalMemoryLimit(t *testing.T) {
	// 8 KB limit: only two 4 KB small slabs can be allocated
	pool := NewPool(8 * 1024)

	b1, err := pool.Acquire(1024)
	if err != nil {
		t.Fatalf("b1 acquire failed: %v", err)
	}
	b2, err := pool.Acquire(1024)
	if err != nil {
		t.Fatalf("b2 acquire failed: %v", err)
	}

	// 3rd acquire must fail with ErrMemoryExhausted
	_, err = pool.Acquire(1024)
	if !errors.Is(err, ErrMemoryExhausted) {
		t.Errorf("expected ErrMemoryExhausted, got %v", err)
	}

	_ = b1.Release()
	_ = b2.Release()
}

func TestMempool_DataZeroingOnRelease(t *testing.T) {
	pool := NewPool(1024 * 1024)

	buf, _ := pool.Acquire(500)
	for i := range buf.Data {
		buf.Data[i] = 0xAA // Fill with non-zero bytes
	}

	_ = buf.Release()

	// Verify buffer data was zeroed on release
	for i, b := range buf.Data {
		if b != 0 {
			t.Fatalf("byte at index %d was not zeroed: %x", i, b)
		}
	}
}

func TestMempool_OversizedRequest(t *testing.T) {
	pool := NewPool(1024 * 1024 * 1024)

	_, err := pool.Acquire(10 * 1024 * 1024) // 10MB > 4MB jumbo slab
	if !errors.Is(err, ErrBufferTooLarge) {
		t.Errorf("expected ErrBufferTooLarge, got %v", err)
	}
}

func TestMempool_Concurrency(t *testing.T) {
	pool := NewPool(100 * 1024 * 1024)
	concurrency := 16
	iterations := 50
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				size := 1024 * ((j % 64) + 1)
				b, err := pool.Acquire(size)
				if err != nil {
					continue
				}
				b.Data[0] = byte(id)
				_ = b.Release()
			}
		}(i)
	}
	wg.Wait()

	used, _ := pool.CurrentUsage()
	if used != 0 {
		t.Errorf("expected 0 bytes used after all workers finished, got %d", used)
	}
}

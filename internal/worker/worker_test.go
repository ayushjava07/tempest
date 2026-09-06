package worker

import (
	"context"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/persistence/memstore"
)

func TestPool_List(t *testing.T) {
	store := memstore.New()
	pool := NewPool(store, PoolOptions{})
	pool.Register(&ExpiryWorker{})
	pool.Register(&CompactionWorker{})
	names := pool.List()
	if len(names) != 2 {
		t.Errorf("expected 2 workers, got %d", len(names))
	}
}

func TestPool_Run_ContextCancel(t *testing.T) {
	store := memstore.New()
	pool := NewPool(store, PoolOptions{})
	pool.Register(&ExpiryWorker{
		store:   store,
		log:     QuietLog{},
		metrics: NopMetrics{},
		clock:   time.Now,
		maxAge:  time.Hour,
		limit:   10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = pool.Run(ctx)
}

func TestExpiryWorker_Name(t *testing.T) {
	w := &ExpiryWorker{}
	if w.Name() != "expiry" {
		t.Errorf("expected expiry, got %s", w.Name())
	}
}

func TestCompactionWorker_Name(t *testing.T) {
	w := &CompactionWorker{}
	if w.Name() != "compaction" {
		t.Errorf("expected compaction, got %s", w.Name())
	}
}

func TestLeaseReaper_Name(t *testing.T) {
	w := &LeaseReaper{}
	if w.Name() != "lease-reaper" {
		t.Errorf("expected lease-reaper, got %s", w.Name())
	}
}

func TestExpiryWorker_NoExpiredRuns(t *testing.T) {
	store := memstore.New()
	w := NewExpiryWorker(store, QuietLog{}, NopMetrics{}, time.Now, ExpiryOptions{})
	w.runOnce(context.Background())
}

func TestCompactionWorker_NoExpiredRuns(t *testing.T) {
	store := memstore.New()
	w := NewCompactionWorker(store, QuietLog{}, NopMetrics{}, time.Now, CompactionOptions{})
	w.runOnce(context.Background())
}

func TestLeaseReaper_NoExpiredLeases(t *testing.T) {
	store := memstore.New()
	w := NewLeaseReaper(store, QuietLog{}, NopMetrics{}, time.Now, LeaseReaperOptions{})
	w.runOnce(context.Background())
}

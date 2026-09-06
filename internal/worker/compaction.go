package worker

import (
	"context"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
)

type CompactionWorker struct {
	store   persistence.Store
	log     Log
	metrics Metrics
	clock   func() time.Time
	before  time.Duration
	limit   int
}

type CompactionOptions struct {
	Before time.Duration
	Limit  int
}

func NewCompactionWorker(store persistence.Store, log Log, metrics Metrics, clock func() time.Time, opts CompactionOptions) *CompactionWorker {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Before <= 0 {
		opts.Before = 30 * 24 * time.Hour
	}
	return &CompactionWorker{
		store:   store,
		log:     log,
		metrics: metrics,
		clock:   clock,
		before:  opts.Before,
		limit:   opts.Limit,
	}
}

func (w *CompactionWorker) Name() string { return "compaction" }

func (w *CompactionWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *CompactionWorker) runOnce(ctx context.Context) {
	before := w.clock().Add(-w.before)
	compacted, err := w.store.CompactRuns(ctx, before, w.limit)
	if err != nil {
		w.log.Warnf("compaction: compact runs: %v", err)
		return
	}
	if compacted > 0 {
		w.metrics.Incr("worker.compaction.compacted")
		w.log.Infof("compaction: compacted %d runs", compacted)
	}
}

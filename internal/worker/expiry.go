package worker

import (
	"context"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
)

type ExpiryWorker struct {
	store   persistence.Store
	log     Log
	metrics Metrics
	clock   func() time.Time
	maxAge  time.Duration
	limit   int
}

type ExpiryOptions struct {
	MaxAge time.Duration
	Limit  int
}

func NewExpiryWorker(store persistence.Store, log Log, metrics Metrics, clock func() time.Time, opts ExpiryOptions) *ExpiryWorker {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.MaxAge <= 0 {
		opts.MaxAge = 7 * 24 * time.Hour
	}
	return &ExpiryWorker{
		store:   store,
		log:     log,
		metrics: metrics,
		clock:   clock,
		maxAge:  opts.MaxAge,
		limit:   opts.Limit,
	}
}

func (w *ExpiryWorker) Name() string { return "expiry" }

func (w *ExpiryWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)
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

func (w *ExpiryWorker) runOnce(ctx context.Context) {
	cutoff := w.clock().Add(-w.maxAge)
	expired, err := w.store.ListExpiredRuns(ctx, cutoff, w.limit)
	if err != nil {
		w.log.Warnf("expiry: list expired runs: %v", err)
		return
	}
	if len(expired) == 0 {
		return
	}
	ids := make([]string, len(expired))
	for i, r := range expired {
		ids[i] = r.ID
	}
	deleted, err := w.store.DeleteRuns(ctx, ids)
	if err != nil {
		w.log.Warnf("expiry: delete runs: %v", err)
		return
	}
	if deleted > 0 {
		w.metrics.Incr("worker.expiry.deleted")
		w.log.Infof("expiry: deleted %d old runs", deleted)
	}
}

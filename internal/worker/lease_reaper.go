package worker

import (
	"context"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
)

type LeaseReaper struct {
	store        persistence.Store
	log          Log
	metrics      Metrics
	clock        func() time.Time
	leaseTimeout time.Duration
	interval     time.Duration
}

type LeaseReaperOptions struct {
	LeaseTimeout time.Duration
	Interval     time.Duration
}

func NewLeaseReaper(store persistence.Store, log Log, metrics Metrics, clock func() time.Time, opts LeaseReaperOptions) *LeaseReaper {
	if opts.LeaseTimeout <= 0 {
		opts.LeaseTimeout = 60 * time.Second
	}
	if opts.Interval <= 0 {
		opts.Interval = 15 * time.Second
	}
	return &LeaseReaper{
		store:        store,
		log:          log,
		metrics:      metrics,
		clock:        clock,
		leaseTimeout: opts.LeaseTimeout,
		interval:     opts.Interval,
	}
}

func (w *LeaseReaper) Name() string { return "lease-reaper" }

func (w *LeaseReaper) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
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

func (w *LeaseReaper) runOnce(ctx context.Context) {
	now := w.clock()
	expired, err := w.store.LeaseExpired(ctx, w.leaseTimeout, now)
	if err != nil {
		w.log.Warnf("lease-reaper: %v", err)
		return
	}
	for _, item := range expired {
		if err := w.store.ReleaseStep(ctx, item.Namespace, item.RunID, item.StepID); err != nil {
			w.log.Warnf("lease-reaper: release step %s/%s: %v", item.RunID, item.StepID, err)
			continue
		}
		w.metrics.Incr("worker.lease_reaper.released")
	}
}

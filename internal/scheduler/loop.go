package scheduler

import (
	"context"
	"sync/atomic"
	"time"
)

type Scheduler struct {
	engine  *Engine
	pool    *Pool
	pollIn  time.Duration
	stopped atomic.Bool
	done    chan struct{}
}

type SchedulerOptions struct {
	PollInterval time.Duration
	Workers      int
}

func NewScheduler(engine *Engine, opts SchedulerOptions) *Scheduler {
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	return &Scheduler{
		engine: engine,
		pool:   NewPool(opts.Workers),
		pollIn: opts.PollInterval,
		done:   make(chan struct{}),
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.pollIn)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.drain(ctx)
			s.pool.Shutdown()
			return
		case <-ticker.C:
			s.drain(ctx)
		}
	}
}

func (s *Scheduler) drain(ctx context.Context) {
	leases, err := s.engine.dequeueLeases(ctx)
	if err != nil {
		s.engine.log.Errorf("drain: dequeue: %v", err)
		return
	}
	for _, lease := range leases {
		lease := lease
		s.pool.SubmitContext(ctx, func() {
			if err := s.engine.processLease(ctx, lease); err != nil {
				s.engine.log.Warnf("process lease %s: %v", lease.Item.StepID, err)
			}
		})
	}
}

func (s *Scheduler) Shutdown() {
	s.stopped.Store(true)
	<-s.done
}

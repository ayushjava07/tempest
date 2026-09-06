package worker

import (
	"context"
	"sync"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
)

type Log interface {
	Errorf(format string, args ...any)
	Warnf(format string, args ...any)
	Infof(format string, args ...any)
}

type QuietLog struct{}

func (QuietLog) Errorf(string, ...any) {}
func (QuietLog) Warnf(string, ...any)  {}
func (QuietLog) Infof(string, ...any)  {}

type Metrics interface {
	Incr(name string)
}

type NopMetrics struct{}

func (NopMetrics) Incr(string) {}

type Worker interface {
	Name() string
	Run(ctx context.Context) error
}

type Pool struct {
	workers  []Worker
	store    persistence.Store
	log      Log
	metrics  Metrics
	clock    func() time.Time
	interval time.Duration
}

type PoolOptions struct {
	Log      Log
	Metrics  Metrics
	Clock    func() time.Time
	Interval time.Duration
}

func NewPool(store persistence.Store, opts PoolOptions) *Pool {
	if opts.Log == nil {
		opts.Log = QuietLog{}
	}
	if opts.Metrics == nil {
		opts.Metrics = NopMetrics{}
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Interval <= 0 {
		opts.Interval = 30 * time.Second
	}
	return &Pool{
		store:    store,
		log:      opts.Log,
		metrics:  opts.Metrics,
		clock:    opts.Clock,
		interval: opts.Interval,
	}
}

func (p *Pool) Register(w Worker) {
	p.workers = append(p.workers, w)
}

func (p *Pool) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(p.workers))
	for _, w := range p.workers {
		wg.Add(1)
		go func(w Worker) {
			defer wg.Done()
			if err := w.Run(ctx); err != nil {
				errCh <- err
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (p *Pool) List() []string {
	names := make([]string, len(p.workers))
	for i, w := range p.workers {
		names[i] = w.Name()
	}
	return names
}

package stealer

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrQueueEmpty = errors.New("stealer: queue is empty")
	ErrQueueFull  = errors.New("stealer: queue capacity exceeded")
	ErrPoolClosed = errors.New("stealer: worker pool is closed")
)

// Task represents an executable work item within the work-stealing pool.
type Task struct {
	ID        string
	Payload   any
	CreatedAt time.Time
}

// Deque implements a double-ended queue optimized for work stealing.
// The local worker pushes and pops from the bottom (LIFO).
// Remote stealers steal from the top (FIFO).
type Deque struct {
	mu       sync.Mutex
	items    []Task
	capacity int
}

func NewDeque(capacity int) *Deque {
	if capacity <= 0 {
		capacity = 256
	}
	return &Deque{
		items:    make([]Task, 0, capacity),
		capacity: capacity,
	}
}

// PushBottom adds a task to the bottom (called exclusively by local owner).
func (d *Deque) PushBottom(t Task) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.items) >= d.capacity {
		return ErrQueueFull
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	d.items = append(d.items, t)
	return nil
}

// PopBottom removes a task from the bottom (called exclusively by local owner).
func (d *Deque) PopBottom() (Task, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	n := len(d.items)
	if n == 0 {
		return Task{}, ErrQueueEmpty
	}

	item := d.items[n-1]
	d.items = d.items[:n-1]
	return item, nil
}

// StealTop steals a task from the top (called by foreign stealers).
func (d *Deque) StealTop() (Task, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.items) == 0 {
		return Task{}, ErrQueueEmpty
	}

	item := d.items[0]
	d.items = d.items[1:]
	return item, nil
}

// Size returns the approximate number of tasks currently queued.
func (d *Deque) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.items)
}

// PoolStats reports metrics for the work-stealing pool.
type PoolStats struct {
	PushedCount uint64
	LocalPopped uint64
	StolenCount uint64
	ActiveTasks int
}

// Worker represents a single thread of execution in the work-stealing pool.
type Worker struct {
	id     int
	deque  *Deque
	pool   *Pool
	stopCh chan struct{}
}

// Handler processes a task.
type Handler func(ctx context.Context, t Task) error

// Pool orchestrates multiple workers with distributed work stealing.
type Pool struct {
	workers []*Worker
	handler Handler
	stats   struct {
		pushed atomic.Uint64
		popped atomic.Uint64
		stolen atomic.Uint64
	}
	wg      sync.WaitGroup
	running atomic.Bool
	stopCh  chan struct{}
}

func NewPool(numWorkers int, dequeCap int, handler Handler) *Pool {
	if numWorkers <= 0 {
		numWorkers = 4
	}
	if dequeCap <= 0 {
		dequeCap = 512
	}

	p := &Pool{
		workers: make([]*Worker, numWorkers),
		handler: handler,
		stopCh:  make(chan struct{}),
	}

	for i := 0; i < numWorkers; i++ {
		p.workers[i] = &Worker{
			id:     i,
			deque:  NewDeque(dequeCap),
			pool:   p,
			stopCh: make(chan struct{}),
		}
	}

	return p
}

// Start boots all worker loops.
func (p *Pool) Start() {
	if p.running.CompareAndSwap(false, true) {
		for _, w := range p.workers {
			p.wg.Add(1)
			go w.run()
		}
	}
}

// Submit enqueues a task onto a target worker's deque.
func (p *Pool) Submit(workerID int, t Task) error {
	if !p.running.Load() {
		return ErrPoolClosed
	}

	idx := workerID % len(p.workers)
	if idx < 0 {
		idx = -idx
	}

	err := p.workers[idx].deque.PushBottom(t)
	if err == nil {
		p.stats.pushed.Add(1)
	}
	return err
}

// Stop gracefully shuts down all workers.
func (p *Pool) Stop() {
	if p.running.CompareAndSwap(true, false) {
		close(p.stopCh)
		for _, w := range p.workers {
			close(w.stopCh)
		}
		p.wg.Wait()
	}
}

// Stats returns a snapshot of pool execution statistics.
func (p *Pool) Stats() PoolStats {
	totalActive := 0
	for _, w := range p.workers {
		totalActive += w.deque.Size()
	}

	return PoolStats{
		PushedCount: p.stats.pushed.Load(),
		LocalPopped: p.stats.popped.Load(),
		StolenCount: p.stats.stolen.Load(),
		ActiveTasks: totalActive,
	}
}

func (w *Worker) run() {
	defer w.pool.wg.Done()

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		// 1. Try to pop local work (LIFO)
		task, err := w.deque.PopBottom()
		if err == nil {
			w.pool.stats.popped.Add(1)
			if w.pool.handler != nil {
				_ = w.pool.handler(context.Background(), task)
			}
			continue
		}

		// 2. Queue empty: try to steal from peers (FIFO)
		stolenTask, err := w.stealFromPeers()
		if err == nil {
			w.pool.stats.stolen.Add(1)
			if w.pool.handler != nil {
				_ = w.pool.handler(context.Background(), stolenTask)
			}
			continue
		}

		// 3. No work available; backoff slightly
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *Worker) stealFromPeers() (Task, error) {
	numWorkers := len(w.pool.workers)
	if numWorkers <= 1 {
		return Task{}, ErrQueueEmpty
	}

	// Pick random starting index to distribute steal requests
	offset := rand.Intn(numWorkers)
	for i := 0; i < numWorkers; i++ {
		targetIdx := (offset + i) % numWorkers
		if targetIdx == w.id {
			continue
		}

		task, err := w.pool.workers[targetIdx].deque.StealTop()
		if err == nil {
			return task, nil
		}
	}

	return Task{}, ErrQueueEmpty
}

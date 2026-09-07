package shaper

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrQueueFull    = errors.New("shaper: queue capacity exceeded")
	ErrShaperClosed = errors.New("shaper: traffic shaper closed")
)

type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
)

type DropPolicy int

const (
	DropNewest DropPolicy = iota
	DropOldest
	BlockUntilSpace
)

// Packet represents a unit of work or payload to be shaped.
type Packet struct {
	ID         string
	Priority   Priority
	Data       []byte
	EnqueuedAt time.Time
	seq        uint64
	heapIdx    int
}

// Priority Queue for packets
type packetQueue []*Packet

func (pq packetQueue) Len() int { return len(pq) }
func (pq packetQueue) Less(i, j int) bool {
	// Higher priority comes first
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// Older sequence comes first (FIFO within same priority)
	return pq[i].seq < pq[j].seq
}
func (pq packetQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].heapIdx = i
	pq[j].heapIdx = j
}
func (pq *packetQueue) Push(x any) {
	p := x.(*Packet)
	p.heapIdx = len(*pq)
	*pq = append(*pq, p)
}
func (pq *packetQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.heapIdx = -1
	*pq = old[0 : n-1]
	return item
}

// Config specifies traffic shaping constraints.
type Config struct {
	Capacity   int
	LeakRate   float64 // packets per second
	DropPolicy DropPolicy
}

// TrafficShaper shapes bursty traffic into a smooth, paced stream.
type TrafficShaper struct {
	mu         sync.Mutex
	cfg        Config
	queue      packetQueue
	paceDelay  time.Duration
	seqCounter uint64
	cond       *sync.Cond
	closed     atomic.Bool

	// Stats
	enqueued atomic.Int64
	drained  atomic.Int64
	dropped  atomic.Int64
}

// New creates a new TrafficShaper.
func New(cfg Config) *TrafficShaper {
	if cfg.Capacity <= 0 {
		cfg.Capacity = 100
	}
	if cfg.LeakRate <= 0 {
		cfg.LeakRate = 100
	}

	delaySec := 1.0 / cfg.LeakRate
	ts := &TrafficShaper{
		cfg:       cfg,
		paceDelay: time.Duration(delaySec * float64(time.Second)),
	}
	ts.cond = sync.NewCond(&ts.mu)
	heap.Init(&ts.queue)
	return ts
}

// Enqueue submits a packet to the traffic shaper.
func (ts *TrafficShaper) Enqueue(ctx context.Context, p Packet) error {
	if ts.closed.Load() {
		return ErrShaperClosed
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	for len(ts.queue) >= ts.cfg.Capacity {
		if ts.closed.Load() {
			return ErrShaperClosed
		}

		switch ts.cfg.DropPolicy {
		case DropNewest:
			ts.dropped.Add(1)
			return ErrQueueFull

		case DropOldest:
			// Find lowest priority oldest element to drop
			lowestIdx := 0
			for i := 1; i < len(ts.queue); i++ {
				if ts.queue[i].Priority < ts.queue[lowestIdx].Priority ||
					(ts.queue[i].Priority == ts.queue[lowestIdx].Priority && ts.queue[i].seq < ts.queue[lowestIdx].seq) {
					lowestIdx = i
				}
			}
			heap.Remove(&ts.queue, lowestIdx)
			ts.dropped.Add(1)

		case BlockUntilSpace:
			// Wait on condition or context
			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					ts.mu.Lock()
					ts.cond.Broadcast()
					ts.mu.Unlock()
				case <-done:
				}
			}()

			ts.cond.Wait()
			close(done)

			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}

	ts.seqCounter++
	p.seq = ts.seqCounter
	p.EnqueuedAt = time.Now()
	heap.Push(&ts.queue, &p)
	ts.enqueued.Add(1)

	ts.cond.Signal()
	return nil
}

// Next pops the next packet according to the paced leak rate and priority.
func (ts *TrafficShaper) Next(ctx context.Context) (Packet, error) {
	ts.mu.Lock()
	for len(ts.queue) == 0 {
		if ts.closed.Load() {
			ts.mu.Unlock()
			return Packet{}, ErrShaperClosed
		}

		ts.cond.Wait()

		if ts.closed.Load() && len(ts.queue) == 0 {
			ts.mu.Unlock()
			return Packet{}, ErrShaperClosed
		}
		if ctx.Err() != nil {
			ts.mu.Unlock()
			return Packet{}, ctx.Err()
		}
	}

	p := heap.Pop(&ts.queue).(*Packet)
	ts.drained.Add(1)
	ts.cond.Broadcast()
	ts.mu.Unlock()

	// Enforce pacing interval
	if ts.paceDelay > 0 {
		select {
		case <-time.After(ts.paceDelay):
		case <-ctx.Done():
			return *p, ctx.Err()
		}
	}

	return *p, nil
}

// Stats returns counters for observability.
func (ts *TrafficShaper) Stats() (queueDepth, enqueued, drained, dropped int64) {
	ts.mu.Lock()
	depth := int64(len(ts.queue))
	ts.mu.Unlock()

	return depth, ts.enqueued.Load(), ts.drained.Load(), ts.dropped.Load()
}

// Close terminates the shaper and releases waiters.
func (ts *TrafficShaper) Close() {
	if !ts.closed.CompareAndSwap(false, true) {
		return
	}
	ts.mu.Lock()
	ts.cond.Broadcast()
	ts.mu.Unlock()
}

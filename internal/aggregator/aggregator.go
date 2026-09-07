package aggregator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrMetricNotFound   = errors.New("aggregator: metric not found")
	ErrInvalidQuantile  = errors.New("aggregator: quantile must be between 0.0 and 1.0")
	ErrEmptyData        = errors.New("aggregator: no data recorded for metric")
	ErrAggregatorClosed = errors.New("aggregator: aggregator is closed")
)

// Reservoir implements memory-bounded reservoir sampling for streaming percentiles.
type Reservoir struct {
	mu       sync.Mutex
	capacity int
	count    int64
	samples  []float64
	sorted   bool
}

// NewReservoir creates a reservoir of fixed sample size.
func NewReservoir(capacity int) *Reservoir {
	if capacity <= 0 {
		capacity = 1024
	}
	return &Reservoir{
		capacity: capacity,
		samples:  make([]float64, 0, capacity),
	}
}

// Add appends a sample using Algorithm R.
func (r *Reservoir) Add(val float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.count++
	r.sorted = false

	if len(r.samples) < r.capacity {
		r.samples = append(r.samples, val)
		return
	}

	// Reservoir full: replace with probability capacity / count
	j := rand.Int63n(r.count)
	if j < int64(r.capacity) {
		r.samples[j] = val
	}
}

// Quantile estimates the value at quantile q [0.0 - 1.0].
func (r *Reservoir) Quantile(q float64) (float64, error) {
	if q < 0.0 || q > 1.0 {
		return 0, ErrInvalidQuantile
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(r.samples)
	if n == 0 {
		return 0, ErrEmptyData
	}

	if !r.sorted {
		sort.Float64s(r.samples)
		r.sorted = true
	}

	if q == 0.0 {
		return r.samples[0], nil
	}
	if q == 1.0 {
		return r.samples[n-1], nil
	}

	idx := q * float64(n-1)
	low := int(math.Floor(idx))
	high := int(math.Ceil(idx))

	if low == high {
		return r.samples[low], nil
	}

	// Linear interpolation
	fraction := idx - float64(low)
	return r.samples[low] + fraction*(r.samples[high]-r.samples[low]), nil
}

// Stats returns summary statistics over the sampled distribution.
func (r *Reservoir) Stats() (count int64, min, max, mean float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.samples) == 0 {
		return 0, 0, 0, 0
	}

	min = r.samples[0]
	max = r.samples[0]
	var sum float64

	for _, v := range r.samples {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
	}

	return r.count, min, max, sum / float64(len(r.samples))
}

// Merge combines another reservoir's samples.
func (r *Reservoir) Merge(other *Reservoir) {
	other.mu.Lock()
	otherSamples := make([]float64, len(other.samples))
	copy(otherSamples, other.samples)
	other.mu.Unlock()

	for _, v := range otherSamples {
		r.Add(v)
	}
}

// TimeBucket holds samples for a discrete time slice.
type TimeBucket struct {
	StartTime time.Time
	EndTime   time.Time
	Count     int64
	Sum       float64
	Reservoir *Reservoir
}

func newTimeBucket(start, end time.Time, reservoirCap int) *TimeBucket {
	return &TimeBucket{
		StartTime: start,
		EndTime:   end,
		Reservoir: NewReservoir(reservoirCap),
	}
}

// SeriesConfig configures the rolling window behavior.
type SeriesConfig struct {
	BucketDuration time.Duration
	WindowDuration time.Duration
	ReservoirSize  int
}

// DefaultSeriesConfig returns 1-minute buckets over a 15-minute window.
func DefaultSeriesConfig() SeriesConfig {
	return SeriesConfig{
		BucketDuration: 1 * time.Minute,
		WindowDuration: 15 * time.Minute,
		ReservoirSize:  512,
	}
}

// RollingSeries maintains a time-windowed sequence of buckets.
type RollingSeries struct {
	mu      sync.RWMutex
	cfg     SeriesConfig
	buckets []*TimeBucket
}

// NewRollingSeries creates a rolling window time series.
func NewRollingSeries(cfg SeriesConfig) *RollingSeries {
	return &RollingSeries{
		cfg:     cfg,
		buckets: make([]*TimeBucket, 0),
	}
}

// Record inserts a data point at time `t`.
func (s *RollingSeries) Record(val float64, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucketStart := t.Truncate(s.cfg.BucketDuration)
	bucketEnd := bucketStart.Add(s.cfg.BucketDuration)

	// Prune expired buckets
	windowCutoff := t.Add(-s.cfg.WindowDuration)
	validIdx := 0
	for i, b := range s.buckets {
		if b.EndTime.After(windowCutoff) {
			validIdx = i
			break
		}
		if i == len(s.buckets)-1 {
			validIdx = len(s.buckets)
		}
	}
	if validIdx > 0 {
		s.buckets = s.buckets[validIdx:]
	}

	// Find or create appropriate bucket
	var target *TimeBucket
	for _, b := range s.buckets {
		if b.StartTime.Equal(bucketStart) {
			target = b
			break
		}
	}

	if target == nil {
		target = newTimeBucket(bucketStart, bucketEnd, s.cfg.ReservoirSize)
		s.buckets = append(s.buckets, target)
		// Keep buckets sorted chronologically
		sort.Slice(s.buckets, func(i, j int) bool {
			return s.buckets[i].StartTime.Before(s.buckets[j].StartTime)
		})
	}

	target.Count++
	target.Sum += val
	target.Reservoir.Add(val)
}

// Snapshot aggregates all active buckets in the window.
func (s *RollingSeries) Snapshot() (*Reservoir, int64, float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	merged := NewReservoir(s.cfg.ReservoirSize * 2)
	var totalCount int64
	var totalSum float64

	for _, b := range s.buckets {
		totalCount += b.Count
		totalSum += b.Sum
		merged.Merge(b.Reservoir)
	}

	return merged, totalCount, totalSum
}

// Aggregator coordinates named metrics across arbitrary labels.
type Aggregator struct {
	mu      sync.RWMutex
	cfg     SeriesConfig
	series  map[string]*RollingSeries
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
}

// New creates a new Aggregator.
func New(cfg SeriesConfig) *Aggregator {
	return &Aggregator{
		cfg:    cfg,
		series: make(map[string]*RollingSeries),
		stopCh: make(chan struct{}),
	}
}

func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(name)
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s=%q", k, labels[k])
	}
	b.WriteString("}")
	return b.String()
}

// Record observes a value for metric `name` with optional labels.
func (a *Aggregator) Record(name string, labels map[string]string, val float64) {
	a.RecordAt(name, labels, val, time.Now())
}

// RecordAt observes a value with an explicit timestamp.
func (a *Aggregator) RecordAt(name string, labels map[string]string, val float64, t time.Time) {
	key := metricKey(name, labels)

	a.mu.Lock()
	s, ok := a.series[key]
	if !ok {
		s = NewRollingSeries(a.cfg)
		a.series[key] = s
	}
	a.mu.Unlock()

	s.Record(val, t)
}

// GetQuantiles computes quantiles (e.g. 0.50, 0.90, 0.99) over the active window.
func (a *Aggregator) GetQuantiles(name string, labels map[string]string, quantiles ...float64) (map[float64]float64, error) {
	key := metricKey(name, labels)

	a.mu.RLock()
	s, ok := a.series[key]
	a.mu.RUnlock()

	if !ok {
		return nil, ErrMetricNotFound
	}

	reservoir, count, _ := s.Snapshot()
	if count == 0 {
		return nil, ErrEmptyData
	}

	results := make(map[float64]float64, len(quantiles))
	for _, q := range quantiles {
		val, err := reservoir.Quantile(q)
		if err != nil {
			return nil, err
		}
		results[q] = val
	}

	return results, nil
}

// Rate computes events per second over the active window.
func (a *Aggregator) Rate(name string, labels map[string]string) (float64, int64, error) {
	key := metricKey(name, labels)

	a.mu.RLock()
	s, ok := a.series[key]
	a.mu.RUnlock()

	if !ok {
		return 0, 0, ErrMetricNotFound
	}

	_, count, _ := s.Snapshot()
	seconds := a.cfg.WindowDuration.Seconds()
	if seconds <= 0 {
		seconds = 1
	}

	return float64(count) / seconds, count, nil
}

// StartSweeper starts a background cleaner for stale buckets.
func (a *Aggregator) StartSweeper(ctx context.Context, interval time.Duration) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-a.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				a.mu.RLock()
				for _, s := range a.series {
					// Trigger pruning by recording zero-delta or touching
					s.mu.Lock()
					windowCutoff := now.Add(-s.cfg.WindowDuration)
					validIdx := 0
					for i, b := range s.buckets {
						if b.EndTime.After(windowCutoff) {
							validIdx = i
							break
						}
						if i == len(s.buckets)-1 {
							validIdx = len(s.buckets)
						}
					}
					if validIdx > 0 {
						s.buckets = s.buckets[validIdx:]
					}
					s.mu.Unlock()
				}
				a.mu.RUnlock()
			}
		}
	}()
}

// Stop gracefully stops the sweeper.
func (a *Aggregator) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	close(a.stopCh)
	a.mu.Unlock()

	a.wg.Wait()
}

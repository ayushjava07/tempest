package promexporter

import (
	"bytes"
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// MetricFamily bundles a registered metric's schema and current sample values.
type MetricFamily struct {
	Name    string
	Help    string
	Type    MetricType
	Samples []Sample
}

// Counter is a monotonically increasing cumulative metric.
type Counter struct {
	name   string
	help   string
	labels map[string]string
	val    atomic.Uint64 // stored as math.Float64bits
}

// NewCounter creates a counter metric.
func NewCounter(name, help string, labels map[string]string) *Counter {
	return &Counter{
		name:   name,
		help:   help,
		labels: labels,
	}
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.Add(1.0)
}

// Add increases the counter by delta (must be non-negative).
func (c *Counter) Add(delta float64) {
	if delta < 0 {
		return
	}
	for {
		oldBits := c.val.Load()
		oldVal := math.Float64frombits(oldBits)
		newVal := oldVal + delta
		newBits := math.Float64bits(newVal)
		if c.val.CompareAndSwap(oldBits, newBits) {
			return
		}
	}
}

// Value returns the current counter value.
func (c *Counter) Value() float64 {
	return math.Float64frombits(c.val.Load())
}

// Gauge represents a numerical value that can arbitrarily go up and down.
type Gauge struct {
	name   string
	help   string
	labels map[string]string
	val    atomic.Uint64
}

// NewGauge creates a gauge metric.
func NewGauge(name, help string, labels map[string]string) *Gauge {
	return &Gauge{
		name:   name,
		help:   help,
		labels: labels,
	}
}

// Set sets the gauge to an arbitrary float64.
func (g *Gauge) Set(val float64) {
	g.val.Store(math.Float64bits(val))
}

// Inc increments gauge by 1.
func (g *Gauge) Inc() {
	g.Add(1.0)
}

// Dec decrements gauge by 1.
func (g *Gauge) Dec() {
	g.Add(-1.0)
}

// Add adjusts the gauge by delta.
func (g *Gauge) Add(delta float64) {
	for {
		oldBits := g.val.Load()
		oldVal := math.Float64frombits(oldBits)
		newVal := oldVal + delta
		newBits := math.Float64bits(newVal)
		if g.val.CompareAndSwap(oldBits, newBits) {
			return
		}
	}
}

// Value returns the current gauge value.
func (g *Gauge) Value() float64 {
	return math.Float64frombits(g.val.Load())
}

// Histogram tracks the statistical distribution of observed events across discrete buckets.
type Histogram struct {
	mu      sync.RWMutex
	name    string
	help    string
	labels  map[string]string
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

// DefaultBuckets are standard HTTP/workflow latency buckets in seconds.
var DefaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewHistogram creates a histogram with specified bucket upper bounds.
func NewHistogram(name, help string, labels map[string]string, buckets []float64) *Histogram {
	if len(buckets) == 0 {
		buckets = DefaultBuckets
	}
	sortedBuckets := make([]float64, len(buckets))
	copy(sortedBuckets, buckets)
	sort.Float64s(sortedBuckets)

	return &Histogram{
		name:    name,
		help:    help,
		labels:  labels,
		buckets: sortedBuckets,
		counts:  make([]uint64, len(sortedBuckets)),
	}
}

// Observe records an observation value.
func (h *Histogram) Observe(val float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += val

	for i, bound := range h.buckets {
		if val <= bound {
			h.counts[i]++
		}
	}
}

// Samples converts the histogram into Prometheus _bucket, _sum, and _count sample rows.
func (h *Histogram) Samples() []Sample {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var samples []Sample

	// Buckets
	for i, bound := range h.buckets {
		lbls := make(map[string]string, len(h.labels)+1)
		for k, v := range h.labels {
			lbls[k] = v
		}
		lbls["le"] = formatFloat(bound)
		samples = append(samples, Sample{
			Name:   h.name + "_bucket",
			Labels: lbls,
			Value:  float64(h.counts[i]),
		})
	}

	// +Inf bucket
	infLbls := make(map[string]string, len(h.labels)+1)
	for k, v := range h.labels {
		infLbls[k] = v
	}
	infLbls["le"] = "+Inf"
	samples = append(samples, Sample{
		Name:   h.name + "_bucket",
		Labels: infLbls,
		Value:  float64(h.count),
	})

	// Sum and Count
	samples = append(samples, Sample{
		Name:   h.name + "_sum",
		Labels: h.labels,
		Value:  h.sum,
	})
	samples = append(samples, Sample{
		Name:   h.name + "_count",
		Labels: h.labels,
		Value:  float64(h.count),
	})

	return samples
}

// CollectorRegistry coordinates telemetry metric collectors.
type CollectorRegistry struct {
	mu         sync.RWMutex
	counters   []*Counter
	gauges     []*Gauge
	histograms []*Histogram
}

// NewRegistry creates a collector registry.
func NewRegistry() *CollectorRegistry {
	return &CollectorRegistry{
		counters:   make([]*Counter, 0),
		gauges:     make([]*Gauge, 0),
		histograms: make([]*Histogram, 0),
	}
}

func (r *CollectorRegistry) RegisterCounter(c *Counter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = append(r.counters, c)
}
func (r *CollectorRegistry) RegisterGauge(g *Gauge) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges = append(r.gauges, g)
}
func (r *CollectorRegistry) RegisterHistogram(h *Histogram) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.histograms = append(r.histograms, h)
}

// Gather formats all registered metrics into Prometheus exposition text.
func (r *CollectorRegistry) Gather() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	buf := new(bytes.Buffer)

	for _, c := range r.counters {
		samples := []Sample{{Name: c.name, Labels: c.labels, Value: c.Value()}}
		SerializeMetricFamily(buf, c.name, c.help, TypeCounter, samples)
	}

	for _, g := range r.gauges {
		samples := []Sample{{Name: g.name, Labels: g.labels, Value: g.Value()}}
		SerializeMetricFamily(buf, g.name, g.help, TypeGauge, samples)
	}

	for _, h := range r.histograms {
		samples := h.Samples()
		SerializeMetricFamily(buf, h.name, h.help, TypeHistogram, samples)
	}

	return buf.Bytes()
}

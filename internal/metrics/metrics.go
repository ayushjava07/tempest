package metrics

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Counter struct {
	name  string
	value atomic.Int64
	tags  map[string]string
}

func (c *Counter) Incr(delta int64) {
	c.value.Add(delta)
}

func (c *Counter) Value() int64 {
	return c.value.Load()
}

type Gauge struct {
	name  string
	value atomic.Int64
	tags  map[string]string
}

func (g *Gauge) Set(value int64) {
	g.value.Store(value)
}

func (g *Gauge) Incr(delta int64) {
	g.value.Add(delta)
}

func (g *Gauge) Value() int64 {
	return g.value.Load()
}

type Histogram struct {
	name   string
	mu     sync.Mutex
	bounds []float64
	counts []int64
	sum    float64
	count  int64
	tags   map[string]string
}

func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += value
	h.count++
	for i, bound := range h.bounds {
		if value <= bound {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.counts)-1]++
}

func (h *Histogram) Snapshot() HistogramSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	counts := make([]int64, len(h.counts))
	copy(counts, h.counts)
	return HistogramSnapshot{
		Name:   h.name,
		Counts: counts,
		Sum:    h.sum,
		Count:  h.count,
		Bounds: h.bounds,
	}
}

type HistogramSnapshot struct {
	Name   string    `json:"name"`
	Counts []int64   `json:"counts"`
	Sum    float64   `json:"sum"`
	Count  int64     `json:"count"`
	Bounds []float64 `json:"bounds"`
}

type Registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
	histos   map[string]*Histogram
	started  time.Time
}

func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*Gauge),
		histos:   make(map[string]*Histogram),
		started:  time.Now(),
	}
}

func (r *Registry) Counter(name string, tags ...string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{name: name, tags: parseTags(tags)}
	r.counters[name] = c
	return c
}

func (r *Registry) Gauge(name string, tags ...string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g := &Gauge{name: name, tags: parseTags(tags)}
	r.gauges[name] = g
	return g
}

func (r *Registry) Histogram(name string, bounds []float64, tags ...string) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histos[name]; ok {
		return h
	}
	sorted := make([]float64, len(bounds))
	copy(sorted, bounds)
	sort.Float64s(sorted)
	h := &Histogram{
		name:   name,
		bounds: sorted,
		counts: make([]int64, len(sorted)+1),
		tags:   parseTags(tags),
	}
	r.histos[name] = h
	return h
}

type Snapshot struct {
	Timestamp  string              `json:"timestamp"`
	Uptime     string              `json:"uptime"`
	Counters   []CounterSnapshot   `json:"counters"`
	Gauges     []GaugeSnapshot     `json:"gauges"`
	Histograms []HistogramSnapshot `json:"histograms"`
}

type CounterSnapshot struct {
	Name  string            `json:"name"`
	Value int64             `json:"value"`
	Tags  map[string]string `json:"tags,omitempty"`
}

type GaugeSnapshot struct {
	Name  string            `json:"name"`
	Value int64             `json:"value"`
	Tags  map[string]string `json:"tags,omitempty"`
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snap := Snapshot{
		Timestamp: time.Now().Format(time.RFC3339),
		Uptime:    time.Since(r.started).String(),
	}
	snap.Counters = make([]CounterSnapshot, 0, len(r.counters))
	for _, c := range r.counters {
		snap.Counters = append(snap.Counters, CounterSnapshot{
			Name: c.name, Value: c.value.Load(), Tags: c.tags,
		})
	}
	snap.Gauges = make([]GaugeSnapshot, 0, len(r.gauges))
	for _, g := range r.gauges {
		snap.Gauges = append(snap.Gauges, GaugeSnapshot{
			Name: g.name, Value: g.value.Load(), Tags: g.tags,
		})
	}
	snap.Histograms = make([]HistogramSnapshot, 0, len(r.histos))
	for _, h := range r.histos {
		snap.Histograms = append(snap.Histograms, h.Snapshot())
	}
	return snap
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		snap := r.Snapshot()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
	})
}

func parseTags(tags []string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags)/2)
	for i := 0; i+1 < len(tags); i += 2 {
		m[tags[i]] = tags[i+1]
	}
	return m
}

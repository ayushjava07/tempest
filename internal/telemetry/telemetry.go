package telemetry

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type MetricType string

const (
	MetricCounter   MetricType = "counter"
	MetricGauge     MetricType = "gauge"
	MetricHistogram MetricType = "histogram"
)

type Metric struct {
	Name       string            `json:"name"`
	Type       MetricType        `json:"type"`
	Value      float64           `json:"value"`
	Labels     map[string]string `json:"labels"`
	Timestamp  time.Time         `json:"timestamp"`
}

type Collector struct {
	mu       sync.RWMutex
	metrics  []Metric
	counters map[string]*atomic.Int64
	gauges   map[string]*atomic.Int64
}

func NewCollector() *Collector {
	return &Collector{
		metrics:  make([]Metric, 0),
		counters: make(map[string]*atomic.Int64),
		gauges:   make(map[string]*atomic.Int64),
	}
}

func (c *Collector) Counter(name string, labels map[string]string) *atomic.Int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := name + labelsKey(labels)
	if c.counters[key] == nil {
		c.counters[key] = &atomic.Int64{}
	}
	return c.counters[key]
}

func (c *Collector) Gauge(name string, labels map[string]string) *atomic.Int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := name + labelsKey(labels)
	if c.gauges[key] == nil {
		c.gauges[key] = &atomic.Int64{}
	}
	return c.gauges[key]
}

func (c *Collector) Record(m Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, m)
}

func (c *Collector) Metrics() []Metric {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]Metric, len(c.metrics))
	copy(result, c.metrics)
	return result
}

func (c *Collector) Snapshot() []Metric {
	return c.Metrics()
}

func labelsKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	var s string
	for k, v := range labels {
		s += k + "=" + v + ","
	}
	return s
}

type Exporter interface {
	Export(ctx context.Context, metrics []Metric) error
}

type StdoutExporter struct{}

func (e *StdoutExporter) Export(ctx context.Context, metrics []Metric) error {
	for _, m := range metrics {
		fmt.Printf("%s: %f %v\n", m.Name, m.Value, m.Labels)
	}
	return nil
}

type PeriodicExporter struct {
	exporter  Exporter
	interval  time.Duration
	collector *Collector
	stopCh    chan struct{}
}

func NewPeriodicExporter(collector *Collector, exporter Exporter, interval time.Duration) *PeriodicExporter {
	return &PeriodicExporter{
		exporter:  exporter,
		interval:  interval,
		collector: collector,
		stopCh:    make(chan struct{}),
	}
}

func (p *PeriodicExporter) Start(ctx context.Context) {
	go p.run(ctx)
}

func (p *PeriodicExporter) run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			metrics := p.collector.Snapshot()
			if len(metrics) > 0 {
				_ = p.exporter.Export(ctx, metrics)
			}
		}
	}
}

func (p *PeriodicExporter) Stop() {
	close(p.stopCh)
}
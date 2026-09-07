package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestCollector_Counter(t *testing.T) {
	c := NewCollector()
	counter := c.Counter("requests", map[string]string{"method": "GET"})
	counter.Add(1)
	counter.Add(1)
	if counter.Load() != 2 {
		t.Errorf("expected 2, got %d", counter.Load())
	}
}

func TestCollector_Gauge(t *testing.T) {
	c := NewCollector()
	gauge := c.Gauge("memory", map[string]string{"unit": "MB"})
	gauge.Store(100)
	if gauge.Load() != 100 {
		t.Errorf("expected 100, got %d", gauge.Load())
	}
	gauge.Add(50)
	if gauge.Load() != 150 {
		t.Errorf("expected 150, got %d", gauge.Load())
	}
}

func TestCollector_Record(t *testing.T) {
	c := NewCollector()
	c.Record(Metric{Name: "test", Type: MetricCounter, Value: 1, Timestamp: time.Now()})
	metrics := c.Metrics()
	if len(metrics) != 1 {
		t.Errorf("expected 1, got %d", len(metrics))
	}
}

func TestCollector_Snapshot(t *testing.T) {
	c := NewCollector()
	c.Record(Metric{Name: "a", Type: MetricCounter, Value: 1, Timestamp: time.Now()})
	snap := c.Snapshot()
	if len(snap) != 1 {
		t.Errorf("expected 1, got %d", len(snap))
	}
}

func TestStdoutExporter(t *testing.T) {
	e := &StdoutExporter{}
	metrics := []Metric{
		{Name: "test", Type: MetricCounter, Value: 1, Labels: map[string]string{"a": "b"}, Timestamp: time.Now()},
	}
	if err := e.Export(context.Background(), metrics); err != nil {
		t.Fatal(err)
	}
}

func TestPeriodicExporter(t *testing.T) {
	c := NewCollector()
	e := &StdoutExporter{}
	p := NewPeriodicExporter(c, e, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	p.Stop()
}
package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCounter(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("requests")
	c.Incr(1)
	c.Incr(5)
	if c.Value() != 6 {
		t.Errorf("expected 6, got %d", c.Value())
	}
}

func TestGauge(t *testing.T) {
	r := NewRegistry()
	g := r.Gauge("connections")
	g.Set(10)
	g.Incr(-3)
	if g.Value() != 7 {
		t.Errorf("expected 7, got %d", g.Value())
	}
}

func TestHistogram(t *testing.T) {
	r := NewRegistry()
	h := r.Histogram("latency", []float64{10, 50, 100, 500})
	h.Observe(5)
	h.Observe(25)
	h.Observe(75)
	h.Observe(200)
	h.Observe(1000)
	snap := h.Snapshot()
	if snap.Count != 5 {
		t.Errorf("expected 5 observations, got %d", snap.Count)
	}
	if snap.Counts[0] != 1 {
		t.Errorf("expected 1 in first bucket, got %d", snap.Counts[0])
	}
}

func TestRegistry_Snapshot(t *testing.T) {
	r := NewRegistry()
	r.Counter("req").Incr(10)
	r.Gauge("conn").Set(5)
	snap := r.Snapshot()
	if len(snap.Counters) != 1 {
		t.Errorf("expected 1 counter, got %d", len(snap.Counters))
	}
	if len(snap.Gauges) != 1 {
		t.Errorf("expected 1 gauge, got %d", len(snap.Gauges))
	}
	if snap.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestRegistry_Handler(t *testing.T) {
	r := NewRegistry()
	r.Counter("test").Incr(1)
	req := httptest.NewRequest("GET", "/debug/metrics", nil)
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var snap Snapshot
	if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(snap.Counters) != 1 {
		t.Errorf("expected 1 counter in response, got %d", len(snap.Counters))
	}
}

func TestCounter_DuplicateReturnsSame(t *testing.T) {
	r := NewRegistry()
	c1 := r.Counter("dup")
	c2 := r.Counter("dup")
	c1.Incr(1)
	if c2.Value() != 1 {
		t.Error("expected same counter instance")
	}
}

func TestHistogram_NoBounds(t *testing.T) {
	r := NewRegistry()
	h := r.Histogram("nobounds", nil)
	h.Observe(42)
	snap := h.Snapshot()
	if snap.Count != 1 {
		t.Errorf("expected 1, got %d", snap.Count)
	}
	if snap.Counts[0] != 1 {
		t.Errorf("expected 1 in overflow bucket, got %d", snap.Counts[0])
	}
}

func TestTags(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("tagged", "env", "prod", "region", "us-east")
	snap := r.Snapshot()
	if len(snap.Counters) != 1 {
		t.Fatal("expected 1 counter")
	}
	if snap.Counters[0].Tags["env"] != "prod" {
		t.Errorf("expected env=prod, got %v", snap.Counters[0].Tags)
	}
	_ = c
}

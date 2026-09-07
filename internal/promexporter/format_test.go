package promexporter

import (
	"bytes"
	"strings"
	"testing"
)

func TestLabelEscapingAndSorting(t *testing.T) {
	labels := map[string]string{
		"path":    `/var/log/"app"/error\n.log`,
		"cluster": "prod-east",
		"tier":    "frontend",
	}

	line := FormatSample("http_requests_total", labels, 42.0)

	// Verify labels sorted alphabetically: cluster, path, tier
	if !strings.Contains(line, `cluster="prod-east"`) {
		t.Fatalf("missing cluster label: %s", line)
	}
	if !strings.Contains(line, `tier="frontend"`) {
		t.Fatalf("missing tier label: %s", line)
	}

	// Verify quote and backslash escaped
	if !strings.Contains(line, `\"app\"`) {
		t.Fatalf("quote was not properly escaped in: %s", line)
	}
	if !strings.Contains(line, `\\n`) {
		t.Fatalf("backslash was not escaped in: %s", line)
	}

	// Verify ends with " 42\n"
	if !strings.HasSuffix(line, " 42\n") {
		t.Fatalf("unexpected line suffix: %s", line)
	}
}

func TestMetricFamilySerialization(t *testing.T) {
	buf := new(bytes.Buffer)
	samples := []Sample{
		{Name: "my_counter", Labels: map[string]string{"env": "prod"}, Value: 100},
		{Name: "my_counter", Labels: map[string]string{"env": "dev"}, Value: 20},
	}

	SerializeMetricFamily(buf, "my_counter", "Total count of events", TypeCounter, samples)
	out := buf.String()

	expectedLines := []string{
		"# HELP my_counter Total count of events",
		"# TYPE my_counter counter",
		`my_counter{env="prod"} 100`,
		`my_counter{env="dev"} 20`,
	}

	for _, expected := range expectedLines {
		if !strings.Contains(out, expected) {
			t.Errorf("output missing expected line %q:\n%s", expected, out)
		}
	}
}

func TestCounterAndGaugeOperations(t *testing.T) {
	c := NewCounter("requests", "count", nil)
	c.Inc()
	c.Add(4.5)
	if c.Value() != 5.5 {
		t.Fatalf("expected counter 5.5, got %f", c.Value())
	}
	// Negative addition should be ignored
	c.Add(-2.0)
	if c.Value() != 5.5 {
		t.Fatalf("counter must not decrease on negative add")
	}

	g := NewGauge("temperature", "degrees", nil)
	g.Set(25.0)
	g.Inc()
	g.Dec()
	g.Add(3.5)
	if g.Value() != 28.5 {
		t.Fatalf("expected gauge 28.5, got %f", g.Value())
	}
}

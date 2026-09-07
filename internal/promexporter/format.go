package promexporter

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MetricType classifies standard Prometheus telemetry instrument primitives.
type MetricType string

const (
	TypeCounter   MetricType = "counter"
	TypeGauge     MetricType = "gauge"
	TypeHistogram MetricType = "histogram"
	TypeSummary   MetricType = "summary"
)

var (
	validMetricNameRegex = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
)

// Sample represents a single labeled measurement line.
type Sample struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

// FormatSample formats a single metric sample line with labels and value.
func FormatSample(name string, labels map[string]string, value float64) string {
	if len(labels) == 0 {
		return fmt.Sprintf("%s %s\n", name, formatFloat(value))
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteString("=\"")
		sb.WriteString(EscapeLabelValue(labels[k]))
		sb.WriteByte('"')
	}
	sb.WriteString("} ")
	sb.WriteString(formatFloat(value))
	sb.WriteByte('\n')

	return sb.String()
}

// EscapeLabelValue escapes backslashes, quotes, and newlines according to Prometheus text exposition spec.
func EscapeLabelValue(val string) string {
	val = strings.ReplaceAll(val, `\`, `\\`)
	val = strings.ReplaceAll(val, `"`, `\"`)
	val = strings.ReplaceAll(val, "\n", `\n`)
	return val
}

// SerializeMetricFamily writes HELP, TYPE, and all sample rows into the buffer.
func SerializeMetricFamily(buf *bytes.Buffer, name, help string, mType MetricType, samples []Sample) {
	if help != "" {
		fmt.Fprintf(buf, "# HELP %s %s\n", name, help)
	}
	fmt.Fprintf(buf, "# TYPE %s %s\n", name, mType)

	for _, s := range samples {
		sampleName := s.Name
		if sampleName == "" {
			sampleName = name
		}
		buf.WriteString(FormatSample(sampleName, s.Labels, s.Value))
	}
}

func formatFloat(val float64) string {
	return strconv.FormatFloat(val, 'f', -1, 64)
}

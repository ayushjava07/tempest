package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTelemetry_W3CTraceParent(t *testing.T) {
	header := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	traceID, spanID, flags, err := ExtractTraceParent(header)
	if err != nil {
		t.Fatalf("ExtractTraceParent failed: %v", err)
	}

	if traceID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("unexpected traceID: %s", traceID.String())
	}
	if spanID.String() != "00f067aa0ba902b7" {
		t.Errorf("unexpected spanID: %s", spanID.String())
	}
	if !flags.IsSampled() {
		t.Errorf("expected sampled flag to be set")
	}

	span := &Span{
		traceID: traceID,
		spanID:  spanID,
		flags:   flags,
	}
	formatted := InjectTraceParent(span)
	if formatted != header {
		t.Errorf("expected roundtrip %s, got %s", header, formatted)
	}

	// Test invalid headers
	invalidHeaders := []string{
		"",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",       // bad version
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",       // all zero trace
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",       // all zero span
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",          // missing fields
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra", // extra fields
		"00-4bf92f3577b34da6a3ce929d0e0e473g-00f067aa0ba902b7-01",       // non-hex trace
	}

	for _, inv := range invalidHeaders {
		_, _, _, err := ExtractTraceParent(inv)
		if err == nil {
			t.Errorf("expected error for invalid header %q, got nil", inv)
		}
	}
}

func TestTelemetry_SpanLifecycle(t *testing.T) {
	exp := NewInMemorySpanExporter()
	proc := NewBatchSpanProcessor(exp, 100, 10, 50*time.Millisecond)
	tracer := NewTracer("test-tracer", alwaysSampler{}, proc)

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "root-operation", SpanKindServer)
	if span == nil {
		t.Fatal("expected non-nil span")
	}

	span.SetAttribute("workflow.id", "wf-12345")
	span.SetAttribute("workflow.attempt", 2)
	span.AddEvent("step_started", map[string]any{"step_id": "step-1"})
	span.SetStatus(StatusOk, "success")

	attrVal, ok := span.GetAttribute("workflow.id")
	if !ok || attrVal != "wf-12345" {
		t.Errorf("expected attribute workflow.id=wf-12345, got %v", attrVal)
	}

	events := span.Events()
	if len(events) != 1 || events[0].Name != "step_started" {
		t.Fatalf("unexpected events: %v", events)
	}

	span.End()

	// Shut down processor to flush all spans
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := proc.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spans))
	}
	if spans[0].Name() != "root-operation" {
		t.Errorf("unexpected span name: %s", spans[0].Name())
	}
	if spans[0].Status().Code != StatusOk {
		t.Errorf("unexpected status code: %v", spans[0].Status().Code)
	}
}

func TestTelemetry_ContextPropagation(t *testing.T) {
	tracer := NewTracer("propagation-test", alwaysSampler{}, nil)

	ctx := context.Background()
	ctx1, parent := tracer.Start(ctx, "parent-span", SpanKindServer)
	if SpanFromContext(ctx1) != parent {
		t.Fatal("parent span not in context")
	}

	_, child := tracer.Start(ctx1, "child-span", SpanKindInternal)
	if child.TraceID() != parent.TraceID() {
		t.Errorf("child span must share parent TraceID: %s != %s", child.TraceID(), parent.TraceID())
	}
	if child.ParentSpanID() != parent.SpanID() {
		t.Errorf("child span parentID must match parent spanID: %s != %s", child.ParentSpanID(), parent.SpanID())
	}
}

func TestTelemetry_Samplers(t *testing.T) {
	traceID := generateTraceID()

	// Always & Never
	if !(alwaysSampler{}).ShouldSample(traceID, "test") {
		t.Error("AlwaysSampler should sample")
	}
	if (neverSampler{}).ShouldSample(traceID, "test") {
		t.Error("NeverSampler should not sample")
	}

	// Ratio
	ratio0 := NewRatioSampler(0.0)
	if ratio0.ShouldSample(traceID, "test") {
		t.Error("Ratio 0.0 should not sample")
	}
	ratio1 := NewRatioSampler(1.0)
	if !ratio1.ShouldSample(traceID, "test") {
		t.Error("Ratio 1.0 should sample")
	}

	// RateLimiting
	rateSampler := NewRateLimitingSampler(5)
	allowed := 0
	for i := 0; i < 10; i++ {
		if rateSampler.ShouldSample(traceID, "test") {
			allowed++
		}
	}
	if allowed > 5 {
		t.Errorf("expected at most 5 allowed tokens, got %d", allowed)
	}
}

func TestTelemetry_BatchSpanProcessor_Concurrency(t *testing.T) {
	exp := NewInMemorySpanExporter()
	proc := NewBatchSpanProcessor(exp, 500, 20, 20*time.Millisecond)
	tracer := NewTracer("concurrent-test", alwaysSampler{}, proc)

	concurrency := 10
	spansPerRoutine := 20

	done := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			for j := 0; j < spansPerRoutine; j++ {
				_, s := tracer.Start(context.Background(), fmt.Sprintf("span-%d-%d", id, j), SpanKindInternal)
				s.SetAttribute("worker_id", id)
				s.End()
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := proc.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	exported := exp.GetSpans()
	if len(exported) != concurrency*spansPerRoutine {
		t.Errorf("expected %d exported spans, got %d", concurrency*spansPerRoutine, len(exported))
	}
}

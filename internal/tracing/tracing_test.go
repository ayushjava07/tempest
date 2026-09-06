package tracing

import (
	"context"
	"fmt"
	"testing"
)

func TestStartSpan(t *testing.T) {
	span, ctx := StartSpan(context.Background(), "test-op")
	if span.Operation != "test-op" {
		t.Errorf("expected test-op, got %s", span.Operation)
	}
	if span.TraceID == "" {
		t.Error("expected non-empty trace ID")
	}
	if span.SpanID == "" {
		t.Error("expected non-empty span ID")
	}
	if span.ParentID != "" {
		t.Error("expected no parent")
	}
	span.Finish()
	if span.EndTime.IsZero() {
		t.Error("expected non-zero end time")
	}
	_ = ctx
}

func TestNestedSpans(t *testing.T) {
	parent, ctx1 := StartSpan(context.Background(), "parent")
	child, ctx2 := StartSpan(ctx1, "child")
	if child.TraceID != parent.TraceID {
		t.Error("expected same trace ID")
	}
	if child.ParentID != parent.SpanID {
		t.Errorf("expected parent span ID %s, got %s", parent.SpanID, child.ParentID)
	}
	child.Finish()
	parent.Finish()
	_ = ctx2
}

func TestSpan_Tags(t *testing.T) {
	span, _ := StartSpan(context.Background(), "tagged")
	span.SetTag("env", "prod")
	span.SetTag("region", "us-east")
	if span.Tags["env"] != "prod" {
		t.Errorf("expected prod, got %s", span.Tags["env"])
	}
}

func TestSpan_Events(t *testing.T) {
	span, _ := StartSpan(context.Background(), "events")
	span.AddEvent("step-started", map[string]string{"step": "s1"})
	span.AddEvent("step-completed", map[string]string{"step": "s1"})
	if len(span.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(span.Events))
	}
}

func TestSpan_SetError(t *testing.T) {
	span, _ := StartSpan(context.Background(), "error")
	span.SetError(fmt.Errorf("boom"))
	if span.Status != "error" {
		t.Errorf("expected error status, got %s", span.Status)
	}
	if span.Tags["error"] != "boom" {
		t.Errorf("expected boom error tag, got %s", span.Tags["error"])
	}
}

func TestSpan_NilError(t *testing.T) {
	span, _ := StartSpan(context.Background(), "ok")
	span.SetError(nil)
	if span.Status != "ok" {
		t.Errorf("expected ok status, got %s", span.Status)
	}
}

func TestTracer(t *testing.T) {
	tracer := NewTracer()
	span1, _ := StartSpan(context.Background(), "op1")
	span1.Finish()
	tracer.Record(span1)
	span2, _ := StartSpan(context.Background(), "op2")
	span2.Finish()
	tracer.Record(span2)
	if len(tracer.Spans()) != 2 {
		t.Errorf("expected 2 spans, got %d", len(tracer.Spans()))
	}
	tracer.Clear()
	if len(tracer.Spans()) != 0 {
		t.Error("expected cleared spans")
	}
}

func TestNewTraceID(t *testing.T) {
	id1 := NewTraceID()
	id2 := NewTraceID()
	if id1 == "" || id2 == "" {
		t.Error("expected non-empty IDs")
	}
	if id1 == id2 {
		t.Error("expected different trace IDs")
	}
}

func TestNewSpanID(t *testing.T) {
	id1 := NewSpanID()
	id2 := NewSpanID()
	if id1 == "" || id2 == "" {
		t.Error("expected non-empty IDs")
	}
	if id1 == id2 {
		t.Error("expected different span IDs")
	}
}

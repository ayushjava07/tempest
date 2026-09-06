package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Span struct {
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Operation string            `json:"operation"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	Status    string            `json:"status"`
	Tags      map[string]string `json:"tags,omitempty"`
	Events    []SpanEvent       `json:"events,omitempty"`
}

type SpanEvent struct {
	Timestamp  time.Time         `json:"timestamp"`
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type spanKey struct{}

func WithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanKey{}, span)
}

func SpanFrom(ctx context.Context) (*Span, bool) {
	s, ok := ctx.Value(spanKey{}).(*Span)
	return s, ok
}

func StartSpan(ctx context.Context, operation string) (*Span, context.Context) {
	traceID := NewTraceID()
	spanID := NewSpanID()
	var parentID string
	if parent, ok := SpanFrom(ctx); ok {
		traceID = parent.TraceID
		parentID = parent.SpanID
	}
	span := &Span{
		TraceID:   traceID,
		SpanID:    spanID,
		ParentID:  parentID,
		Operation: operation,
		StartTime: time.Now(),
		Status:    "ok",
		Tags:      make(map[string]string),
	}
	return span, WithSpan(ctx, span)
}

func (s *Span) Finish() {
	s.EndTime = time.Now()
}

func (s *Span) SetTag(key, value string) {
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

func (s *Span) AddEvent(name string, attrs map[string]string) {
	s.Events = append(s.Events, SpanEvent{
		Timestamp:  time.Now(),
		Name:       name,
		Attributes: attrs,
	})
}

func (s *Span) SetError(err error) {
	if err != nil {
		s.Status = "error"
		if s.Tags == nil {
			s.Tags = make(map[string]string)
		}
		s.Tags["error"] = err.Error()
	}
}

func NewTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Tracer struct {
	spans []Span
}

func NewTracer() *Tracer {
	return &Tracer{}
}

func (t *Tracer) Record(span *Span) {
	t.spans = append(t.spans, *span)
}

func (t *Tracer) Spans() []Span {
	out := make([]Span, len(t.spans))
	copy(out, t.spans)
	return out
}

func (t *Tracer) Clear() {
	t.spans = nil
}

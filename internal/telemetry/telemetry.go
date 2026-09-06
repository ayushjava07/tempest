package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInvalidTraceParent   = errors.New("telemetry: invalid W3C traceparent header")
	ErrAllZeroTraceID       = errors.New("telemetry: all-zero trace-id is forbidden by W3C spec")
	ErrAllZeroSpanID        = errors.New("telemetry: all-zero span-id is forbidden by W3C spec")
	ErrBatchProcessorClosed = errors.New("telemetry: batch span processor is closed")
)

// TraceID is a 16-byte (128-bit) unique identifier for a trace.
type TraceID [16]byte

func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

func (t TraceID) IsValid() bool {
	for _, b := range t {
		if b != 0 {
			return true
		}
	}
	return false
}

// SpanID is an 8-byte (64-bit) unique identifier for a span.
type SpanID [8]byte

func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

func (s SpanID) IsValid() bool {
	for _, b := range s {
		if b != 0 {
			return true
		}
	}
	return false
}

// TraceFlags represents 8-bit W3C trace flags (e.g. sampled flag 0x01).
type TraceFlags byte

const (
	FlagSampled TraceFlags = 0x01
)

func (f TraceFlags) IsSampled() bool {
	return (f & FlagSampled) == FlagSampled
}

// SpanKind specifies the role of a span in a distributed trace.
type SpanKind string

const (
	SpanKindInternal SpanKind = "internal"
	SpanKindServer   SpanKind = "server"
	SpanKindClient   SpanKind = "client"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
)

// StatusCode represents standard span completion status.
type StatusCode int

const (
	StatusUnset StatusCode = 0
	StatusOk    StatusCode = 1
	StatusError StatusCode = 2
)

// SpanStatus encapsulates status code and an optional descriptive error message.
type SpanStatus struct {
	Code        StatusCode
	Description string
}

// SpanEvent represents a single timestamped annotation within a span.
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]any
}

// SpanLink represents a causal link to another span.
type SpanLink struct {
	TraceID    TraceID
	SpanID     SpanID
	TraceFlags TraceFlags
	Attributes map[string]any
}

// Span represents a single unit of work within a distributed trace.
type Span struct {
	mu           sync.RWMutex
	traceID      TraceID
	spanID       SpanID
	parentSpanID SpanID
	name         string
	kind         SpanKind
	flags        TraceFlags
	startTime    time.Time
	endTime      time.Time
	status       SpanStatus
	attributes   map[string]any
	events       []SpanEvent
	links        []SpanLink
	tracer       *Tracer
	ended        atomic.Bool
}

func (s *Span) TraceID() TraceID     { return s.traceID }
func (s *Span) SpanID() SpanID       { return s.spanID }
func (s *Span) ParentSpanID() SpanID { return s.parentSpanID }
func (s *Span) Name() string         { s.mu.RLock(); defer s.mu.RUnlock(); return s.name }
func (s *Span) Kind() SpanKind       { return s.kind }
func (s *Span) Flags() TraceFlags    { return s.flags }
func (s *Span) StartTime() time.Time { return s.startTime }
func (s *Span) EndTime() time.Time   { s.mu.RLock(); defer s.mu.RUnlock(); return s.endTime }
func (s *Span) Status() SpanStatus   { s.mu.RLock(); defer s.mu.RUnlock(); return s.status }
func (s *Span) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endTime.Sub(s.startTime)
}

func (s *Span) SetAttribute(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]any)
	}
	s.attributes[key] = val
}

func (s *Span) GetAttribute(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.attributes == nil {
		return nil, false
	}
	val, ok := s.attributes[key]
	return val, ok
}

func (s *Span) Attributes() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]any, len(s.attributes))
	for k, v := range s.attributes {
		cp[k] = v
	}
	return cp
}

func (s *Span) AddEvent(name string, attrs map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := SpanEvent{
		Name:       name,
		Timestamp:  time.Now().UTC(),
		Attributes: make(map[string]any),
	}
	for k, v := range attrs {
		ev.Attributes[k] = v
	}
	s.events = append(s.events, ev)
}

func (s *Span) Events() []SpanEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SpanEvent, len(s.events))
	copy(out, s.events)
	return out
}

func (s *Span) SetStatus(code StatusCode, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = SpanStatus{Code: code, Description: description}
}

func (s *Span) End() {
	if s.ended.CompareAndSwap(false, true) {
		s.mu.Lock()
		if s.endTime.IsZero() {
			s.endTime = time.Now().UTC()
		}
		s.mu.Unlock()

		if s.tracer != nil && s.tracer.processor != nil {
			s.tracer.processor.OnEnd(s)
		}
	}
}

// Sampler decides whether a span should be sampled for recording and exporting.
type Sampler interface {
	ShouldSample(traceID TraceID, name string) bool
}

type alwaysSampler struct{}

func (alwaysSampler) ShouldSample(TraceID, string) bool { return true }

type neverSampler struct{}

func (neverSampler) ShouldSample(TraceID, string) bool { return false }

type ratioSampler struct {
	threshold uint64
}

func NewRatioSampler(ratio float64) Sampler {
	if ratio <= 0.0 {
		return neverSampler{}
	}
	if ratio >= 1.0 {
		return alwaysSampler{}
	}
	threshold := uint64(ratio * float64(math.MaxUint64))
	return &ratioSampler{threshold: threshold}
}

func (r *ratioSampler) ShouldSample(traceID TraceID, _ string) bool {
	// Use lower 8 bytes of TraceID as random seed
	var val uint64
	for i := 8; i < 16; i++ {
		val = (val << 8) | uint64(traceID[i])
	}
	return val < r.threshold
}

// RateLimitingSampler limits span creation to maxSpansPerSecond.
type RateLimitingSampler struct {
	mu         sync.Mutex
	maxPerSec  int
	tokens     float64
	lastRefill time.Time
}

func NewRateLimitingSampler(maxPerSec int) *RateLimitingSampler {
	return &RateLimitingSampler{
		maxPerSec:  maxPerSec,
		tokens:     float64(maxPerSec),
		lastRefill: time.Now(),
	}
}

func (r *RateLimitingSampler) ShouldSample(TraceID, string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.lastRefill = now
	r.tokens += elapsed * float64(r.maxPerSec)
	if r.tokens > float64(r.maxPerSec) {
		r.tokens = float64(r.maxPerSec)
	}

	if r.tokens >= 1.0 {
		r.tokens -= 1.0
		return true
	}
	return false
}

// SpanExporter exports batches of ended spans.
type SpanExporter interface {
	ExportSpans(ctx context.Context, spans []*Span) error
	Shutdown(ctx context.Context) error
}

// InMemorySpanExporter retains spans in memory for unit testing and validation.
type InMemorySpanExporter struct {
	mu       sync.Mutex
	spans    []*Span
	isClosed bool
}

func NewInMemorySpanExporter() *InMemorySpanExporter {
	return &InMemorySpanExporter{}
}

func (e *InMemorySpanExporter) ExportSpans(ctx context.Context, spans []*Span) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.isClosed {
		return errors.New("exporter is closed")
	}
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *InMemorySpanExporter) GetSpans() []*Span {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*Span, len(e.spans))
	copy(out, e.spans)
	return out
}

func (e *InMemorySpanExporter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = nil
}

func (e *InMemorySpanExporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.isClosed = true
	return nil
}

// SpanProcessor handles lifecycle notifications of spans.
type SpanProcessor interface {
	OnStart(s *Span)
	OnEnd(s *Span)
	Shutdown(ctx context.Context) error
	ForceFlush(ctx context.Context) error
}

// BatchSpanProcessor buffers spans in memory and flushes in batches.
type BatchSpanProcessor struct {
	exporter     SpanExporter
	queue        chan *Span
	batchSize    int
	flushTimeout time.Duration
	done         chan struct{}
	wg           sync.WaitGroup
	closed       atomic.Bool
}

func NewBatchSpanProcessor(exporter SpanExporter, maxQueueSize, batchSize int, flushTimeout time.Duration) *BatchSpanProcessor {
	if batchSize <= 0 {
		batchSize = 128
	}
	if maxQueueSize < batchSize {
		maxQueueSize = batchSize * 4
	}
	if flushTimeout <= 0 {
		flushTimeout = 200 * time.Millisecond
	}

	b := &BatchSpanProcessor{
		exporter:     exporter,
		queue:        make(chan *Span, maxQueueSize),
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		done:         make(chan struct{}),
	}

	b.wg.Add(1)
	go b.worker()
	return b
}

func (b *BatchSpanProcessor) OnStart(s *Span) {}

func (b *BatchSpanProcessor) OnEnd(s *Span) {
	if b.closed.Load() {
		return
	}
	select {
	case b.queue <- s:
	default:
		// Queue full, drop span gracefully to avoid blocking callers
	}
}

func (b *BatchSpanProcessor) worker() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.flushTimeout)
	defer ticker.Stop()

	batch := make([]*Span, 0, b.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = b.exporter.ExportSpans(ctx, batch)
		cancel()
		batch = make([]*Span, 0, b.batchSize)
	}

	for {
		select {
		case <-b.done:
			// Drain remaining spans
			for {
				select {
				case s := <-b.queue:
					batch = append(batch, s)
					if len(batch) >= b.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case s := <-b.queue:
			batch = append(batch, s)
			if len(batch) >= b.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (b *BatchSpanProcessor) ForceFlush(ctx context.Context) error {
	// Simple flush implementation waits for queue to empty
	timeout := time.After(b.flushTimeout * 2)
	for len(b.queue) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("force flush timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil
}

func (b *BatchSpanProcessor) Shutdown(ctx context.Context) error {
	if b.closed.CompareAndSwap(false, true) {
		close(b.done)
		b.wg.Wait()
		return b.exporter.Shutdown(ctx)
	}
	return nil
}

// Tracer manages span creation and tracing context.
type Tracer struct {
	name      string
	sampler   Sampler
	processor SpanProcessor
}

func NewTracer(name string, sampler Sampler, processor SpanProcessor) *Tracer {
	if sampler == nil {
		sampler = alwaysSampler{}
	}
	return &Tracer{
		name:      name,
		sampler:   sampler,
		processor: processor,
	}
}

type spanKeyType struct{}

var activeSpanKey = spanKeyType{}

// ContextWithSpan returns a new context containing the specified span.
func ContextWithSpan(ctx context.Context, s *Span) context.Context {
	return context.WithValue(ctx, activeSpanKey, s)
}

// SpanFromContext retrieves the currently active span, or nil if none exists.
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	if val := ctx.Value(activeSpanKey); val != nil {
		if s, ok := val.(*Span); ok {
			return s
		}
	}
	return nil
}

// Start creates and activates a new child span.
func (t *Tracer) Start(ctx context.Context, name string, kind SpanKind) (context.Context, *Span) {
	var traceID TraceID
	var parentSpanID SpanID
	var flags TraceFlags

	parent := SpanFromContext(ctx)
	if parent != nil {
		traceID = parent.TraceID()
		parentSpanID = parent.SpanID()
		flags = parent.Flags()
	} else {
		traceID = generateTraceID()
		if t.sampler.ShouldSample(traceID, name) {
			flags |= FlagSampled
		}
	}

	spanID := generateSpanID()

	span := &Span{
		traceID:      traceID,
		spanID:       spanID,
		parentSpanID: parentSpanID,
		name:         name,
		kind:         kind,
		flags:        flags,
		startTime:    time.Now().UTC(),
		attributes:   make(map[string]any),
		tracer:       t,
	}

	if t.processor != nil {
		t.processor.OnStart(span)
	}

	return ContextWithSpan(ctx, span), span
}

func generateTraceID() TraceID {
	var id TraceID
	for {
		_, _ = rand.Read(id[:])
		if id.IsValid() {
			return id
		}
	}
}

func generateSpanID() SpanID {
	var id SpanID
	for {
		_, _ = rand.Read(id[:])
		if id.IsValid() {
			return id
		}
	}
}

// W3C TraceContext Propagator
// Header specification: traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01

// InjectTraceParent formats the active span as a W3C traceparent string.
func InjectTraceParent(s *Span) string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%02x", s.TraceID().String(), s.SpanID().String(), s.Flags())
}

// ExtractTraceParent parses a W3C traceparent header string.
func ExtractTraceParent(header string) (TraceID, SpanID, TraceFlags, error) {
	header = strings.TrimSpace(header)
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return TraceID{}, SpanID{}, 0, ErrInvalidTraceParent
	}

	version := parts[0]
	if version != "00" {
		// Only version 00 is supported according to W3C recommendation
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: unsupported version %s", ErrInvalidTraceParent, version)
	}

	traceIDHex := parts[1]
	if len(traceIDHex) != 32 {
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: trace-id must be 32 hex characters", ErrInvalidTraceParent)
	}
	traceIDBytes, err := hex.DecodeString(traceIDHex)
	if err != nil {
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: invalid hex trace-id", ErrInvalidTraceParent)
	}
	var traceID TraceID
	copy(traceID[:], traceIDBytes)
	if !traceID.IsValid() {
		return TraceID{}, SpanID{}, 0, ErrAllZeroTraceID
	}

	spanIDHex := parts[2]
	if len(spanIDHex) != 16 {
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: span-id must be 16 hex characters", ErrInvalidTraceParent)
	}
	spanIDBytes, err := hex.DecodeString(spanIDHex)
	if err != nil {
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: invalid hex span-id", ErrInvalidTraceParent)
	}
	var spanID SpanID
	copy(spanID[:], spanIDBytes)
	if !spanID.IsValid() {
		return TraceID{}, SpanID{}, 0, ErrAllZeroSpanID
	}

	flagsHex := parts[3]
	if len(flagsHex) != 2 {
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: trace-flags must be 2 hex characters", ErrInvalidTraceParent)
	}
	flagsBytes, err := hex.DecodeString(flagsHex)
	if err != nil {
		return TraceID{}, SpanID{}, 0, fmt.Errorf("%w: invalid hex trace-flags", ErrInvalidTraceParent)
	}

	return traceID, spanID, TraceFlags(flagsBytes[0]), nil
}

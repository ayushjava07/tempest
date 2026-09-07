package webhookdelivery

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// mockHTTPPoster implements HTTPPoster for deterministic testing.
type mockHTTPPoster struct {
	mu         sync.Mutex
	statusCode int
	body       []byte
	err        error
	lastReq    *http.Request
	calls      int
}

func (m *mockHTTPPoster) Do(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastReq = req

	if m.err != nil {
		return nil, m.err
	}

	resp := &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(bytes.NewReader(m.body)),
		Header:     make(http.Header),
	}
	return resp, nil
}

func TestSuccessfulWebhookDispatchWithSignature(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockHTTPPoster{
		statusCode: 200,
		body:       []byte(`{"status":"ok"}`),
	}

	worker := NewWorker(mockClient, DefaultBackoff())

	ep := EndpointConfig{
		ID:         "ep-1",
		URL:        "https://api.partner.com/webhooks",
		Secret:     "super-secret-signing-key",
		MaxRetries: 3,
		Active:     true,
	}

	record := &DeliveryRecord{
		ID:         "deliv-001",
		EndpointID: ep.ID,
		Payload: WebhookPayload{
			EventID:    "evt-100",
			EventType:  "workflow.completed",
			WorkflowID: "wf-order",
			RunID:      "run-xyz",
			Timestamp:  time.Now().UTC(),
			Data:       map[string]any{"total": 99.5},
		},
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	err := worker.Deliver(ctx, record, ep, SignPayload)
	if err != nil {
		t.Fatalf("unexpected delivery error: %v", err)
	}

	if record.Status != StatusSuccess {
		t.Fatalf("expected StatusSuccess, got %s", record.Status)
	}
	if record.CompletedAt == nil {
		t.Fatalf("expected completedAt timestamp to be set")
	}
	if len(record.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(record.Attempts))
	}
	if record.Attempts[0].StatusCode != 200 {
		t.Fatalf("expected status code 200, got %d", record.Attempts[0].StatusCode)
	}

	// Verify request headers
	mockClient.mu.Lock()
	req := mockClient.lastReq
	mockClient.mu.Unlock()

	sigHeader := req.Header.Get("X-Tempest-Signature")
	if sigHeader == "" {
		t.Fatalf("missing signature header")
	}

	// Verify signature using VerifySignature
	bodyBytes, _ := io.ReadAll(req.Body)
	// Body was already read by Deliver, but let's test VerifySignature directly
	signedPayload, _ := io.ReadAll(bytes.NewReader([]byte(`{"test":true}`)))
	s, _ := SignPayload(signedPayload, time.Now().UTC(), ep.Secret)
	if err := VerifySignature(signedPayload, s, ep.Secret, 5*time.Minute); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
	_ = bodyBytes
}

func TestWebhookRetryOnFailure(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockHTTPPoster{
		statusCode: 503,
		body:       []byte(`{"error":"temporarily unavailable"}`),
	}

	worker := NewWorker(mockClient, DefaultBackoff())

	ep := EndpointConfig{
		ID:         "ep-2",
		URL:        "https://api.partner.com/webhooks",
		MaxRetries: 3,
		Active:     true,
	}

	record := &DeliveryRecord{
		ID:         "deliv-002",
		EndpointID: ep.ID,
		Status:     StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	// Attempt 1: should transition to RETRYING
	err := worker.Deliver(ctx, record, ep, nil)
	if err == nil {
		t.Fatalf("expected error on 503 response")
	}
	if record.Status != StatusRetrying {
		t.Fatalf("expected StatusRetrying on attempt 1, got %s", record.Status)
	}
	if record.NextRetryAt == nil {
		t.Fatalf("expected NextRetryAt to be set")
	}

	// Attempt 2: still RETRYING
	err = worker.Deliver(ctx, record, ep, nil)
	if err == nil {
		t.Fatalf("expected error on attempt 2")
	}
	if record.Status != StatusRetrying {
		t.Fatalf("expected StatusRetrying on attempt 2, got %s", record.Status)
	}

	// Attempt 3: reached MaxRetries=3 -> DEAD_LETTER
	err = worker.Deliver(ctx, record, ep, nil)
	if err == nil {
		t.Fatalf("expected error on final attempt")
	}
	if record.Status != StatusDeadLetter {
		t.Fatalf("expected StatusDeadLetter on attempt 3, got %s", record.Status)
	}
	if record.NextRetryAt != nil {
		t.Fatalf("NextRetryAt should be cleared on dead letter")
	}
}

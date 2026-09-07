package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWebhook_SignedDispatch(t *testing.T) {
	secret := "super-secret-key-123"
	var receivedEvent Event
	var receivedSig string

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			receivedSig = r.Header.Get("X-Tempest-Signature-256")
			body, _ := io.ReadAll(r.Body)

			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(body)
			expectedSig := hex.EncodeToString(mac.Sum(nil))

			if receivedSig != expectedSig {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			}

			_ = json.Unmarshal(body, &receivedEvent)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"ok"}`))),
			}, nil
		}),
	}

	ch := NewWebhookChannel(WebhookConfig{
		URL:        "http://webhook.internal/events",
		SecretKey:  secret,
		HTTPClient: client,
	})

	ev := Event{
		ID:         "ev-1",
		WorkflowID: "wf-etl",
		RunID:      "run-99",
		EventType:  "FAILED",
		Severity:   SeverityError,
		Title:      "ETL Stage Failed",
		Message:    "Out of disk space on node 4",
		Timestamp:  time.Now().UTC(),
	}

	err := ch.Send(context.Background(), ev)
	if err != nil {
		t.Fatalf("webhook send failed: %v", err)
	}

	if receivedEvent.ID != ev.ID || receivedEvent.WorkflowID != ev.WorkflowID {
		t.Fatalf("received event mismatch: %+v", receivedEvent)
	}
}

func TestSlack_FormatDispatch(t *testing.T) {
	var receivedSlack SlackBlockPayload

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedSlack)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`ok`))),
			}, nil
		}),
	}

	ch := NewSlackChannel("http://slack.internal/hook", client)

	ev := Event{
		ID:         "ev-slack",
		WorkflowID: "deploy-prod",
		RunID:      "run-101",
		EventType:  "APPROVAL_PENDING",
		Severity:   SeverityWarning,
		Title:      "Deployment Approval Required",
		Message:    "Please sign off release v2.4",
		Timestamp:  time.Now().UTC(),
		Metadata:   map[string]string{"Approver": "alice"},
	}

	err := ch.Send(context.Background(), ev)
	if err != nil {
		t.Fatalf("slack send failed: %v", err)
	}

	if len(receivedSlack.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(receivedSlack.Attachments))
	}
	att := receivedSlack.Attachments[0]
	if att.Color != "#ecaa38" { // Warning color
		t.Fatalf("expected warning color #ecaa38, got %s", att.Color)
	}
}

func TestPagerDuty_Dispatch(t *testing.T) {
	var receivedPD PagerDutyEvent

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedPD)
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"accepted"}`))),
			}, nil
		}),
	}

	ch := NewPagerDutyChannel("pd-route-key", client)
	ch.apiURL = "http://pagerduty.internal/v2/enqueue"

	ev := Event{
		ID:         "ev-pd",
		WorkflowID: "payments-engine",
		RunID:      "run-77",
		EventType:  "FAILED",
		Severity:   SeverityCritical,
		Title:      "Payment Gateway Unreachable",
		Message:    "All payment retries exhausted",
		Timestamp:  time.Now().UTC(),
	}

	err := ch.Send(context.Background(), ev)
	if err != nil {
		t.Fatalf("pagerduty send failed: %v", err)
	}

	if receivedPD.EventAction != "trigger" {
		t.Fatalf("expected trigger action, got %s", receivedPD.EventAction)
	}
	if receivedPD.Payload.Severity != "critical" {
		t.Fatalf("expected critical severity, got %s", receivedPD.Payload.Severity)
	}
}

type mockChannel struct {
	sent atomic.Int32
}

func (m *mockChannel) Name() string { return "mock" }
func (m *mockChannel) Send(ctx context.Context, ev Event) error {
	m.sent.Add(1)
	return nil
}

func TestDispatcher_AntiStormSuppression(t *testing.T) {
	mock := &mockChannel{}
	disp := NewDispatcher(AntiStormConfig{
		CooldownWindow: 200 * time.Millisecond,
		MaxBurst:       3,
	})
	disp.RegisterChannel(mock)

	ev := Event{
		WorkflowID: "fragile-job",
		EventType:  "FAILED",
		Severity:   SeverityError,
	}

	// Send 10 rapid alerts
	for i := 0; i < 10; i++ {
		_ = disp.Dispatch(context.Background(), ev)
	}

	// Only MaxBurst (3) should have passed through
	if mock.sent.Load() != 3 {
		t.Fatalf("expected 3 dispatched alerts due to storm suppression, got %d", mock.sent.Load())
	}

	// Wait for cooldown to expire
	time.Sleep(250 * time.Millisecond)

	// Next alert should pass through
	_ = disp.Dispatch(context.Background(), ev)
	if mock.sent.Load() != 4 {
		t.Fatalf("expected 4 dispatched alerts after cooldown reset, got %d", mock.sent.Load())
	}
}

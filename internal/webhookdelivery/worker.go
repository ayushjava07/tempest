package webhookdelivery

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"time"
)

// HTTPPoster abstracts HTTP client execution for testing and connection reuse.
type HTTPPoster interface {
	Do(req *http.Request) (*http.Response, error)
}

// BackoffConfig governs exponential retry spacing and jitter.
type BackoffConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	JitterFraction  float64
}

// DefaultBackoff returns standard webhook retry intervals.
func DefaultBackoff() BackoffConfig {
	return BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     1 * time.Hour,
		Multiplier:      2.0,
		JitterFraction:  0.2,
	}
}

// CalculateBackoff determines retry delay for a given attempt count.
func CalculateBackoff(attempt int, cfg BackoffConfig) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	raw := float64(cfg.InitialInterval) * math.Pow(cfg.Multiplier, float64(attempt-1))
	if raw > float64(cfg.MaxInterval) {
		raw = float64(cfg.MaxInterval)
	}

	jitterRange := raw * cfg.JitterFraction
	if jitterRange <= 0 {
		return time.Duration(raw)
	}

	maxBig := big.NewInt(int64(2 * jitterRange))
	n, err := rand.Int(rand.Reader, maxBig)
	if err != nil {
		return time.Duration(raw)
	}
	offset := float64(n.Int64()) - jitterRange
	delay := time.Duration(raw + offset)
	if delay < 0 {
		delay = cfg.InitialInterval
	}
	return delay
}

// DeliveryWorker executes webhook transmissions with retry tracking.
type DeliveryWorker struct {
	client  HTTPPoster
	backoff BackoffConfig
}

// NewWorker initializes a delivery worker.
func NewWorker(client HTTPPoster, backoff BackoffConfig) *DeliveryWorker {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if backoff.InitialInterval <= 0 {
		backoff = DefaultBackoff()
	}
	return &DeliveryWorker{
		client:  client,
		backoff: backoff,
	}
}

// Deliver executes an HTTP POST attempt for the given record against target endpoint.
func (w *DeliveryWorker) Deliver(ctx context.Context, record *DeliveryRecord, ep EndpointConfig, signer func([]byte, time.Time, string) (string, string)) error {
	payloadBytes, err := json.Marshal(record.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	attemptNum := len(record.Attempts) + 1
	startTime := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Tempest-Webhook-Worker/1.0")

	// Custom configured headers
	for k, v := range ep.Headers {
		req.Header.Set(k, v)
	}

	// Sign payload if signer provided
	now := time.Now().UTC()
	if signer != nil && ep.Secret != "" {
		sig, tsHeader := signer(payloadBytes, now, ep.Secret)
		req.Header.Set("X-Tempest-Signature", sig)
		req.Header.Set("X-Tempest-Timestamp", tsHeader)
	}

	resp, doErr := w.client.Do(req)
	duration := time.Since(startTime)

	statusCode := 0
	errMsg := ""
	if doErr != nil {
		errMsg = doErr.Error()
	} else {
		statusCode = resp.StatusCode
		_ = resp.Body.Close()
	}

	attempt := DeliveryAttempt{
		AttemptNumber: attemptNum,
		Timestamp:     startTime,
		StatusCode:    statusCode,
		Duration:      duration,
		Error:         errMsg,
	}
	record.Attempts = append(record.Attempts, attempt)

	// Evaluate success: 2xx response
	if statusCode >= 200 && statusCode < 300 {
		record.Status = StatusSuccess
		compTime := time.Now().UTC()
		record.CompletedAt = &compTime
		record.NextRetryAt = nil
		return nil
	}

	// Failed attempt: check retries
	if attemptNum >= ep.MaxRetries {
		record.Status = StatusDeadLetter
		compTime := time.Now().UTC()
		record.CompletedAt = &compTime
		record.NextRetryAt = nil
		return fmt.Errorf("delivery exhausted all %d attempts: status %d (%s)", ep.MaxRetries, statusCode, errMsg)
	}

	record.Status = StatusRetrying
	delay := CalculateBackoff(attemptNum, w.backoff)
	nextRetry := time.Now().UTC().Add(delay)
	record.NextRetryAt = &nextRetry

	return fmt.Errorf("delivery failed (attempt %d/%d): status %d (%s)", attemptNum, ep.MaxRetries, statusCode, errMsg)
}

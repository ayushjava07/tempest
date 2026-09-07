package webhookdelivery

import (
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"
)

var (
	ErrInvalidURL       = errors.New("webhook: invalid target url")
	ErrEndpointNotFound = errors.New("webhook: endpoint not found")
	ErrEndpointDisabled = errors.New("webhook: endpoint is disabled")
)

// DeliveryStatus represents the delivery lifecycle state of a webhook notification.
type DeliveryStatus string

const (
	StatusPending    DeliveryStatus = "PENDING"
	StatusSuccess    DeliveryStatus = "SUCCESS"
	StatusRetrying   DeliveryStatus = "RETRYING"
	StatusDeadLetter DeliveryStatus = "DEAD_LETTER"
)

// EndpointConfig defines subscription configuration for a webhook receiver.
type EndpointConfig struct {
	ID         string            `json:"id"`
	URL        string            `json:"url"`
	Secret     string            `json:"secret"` // Shared secret for HMAC-SHA512
	Events     []string          `json:"events"` // Subscribed event types (or "*" for all)
	MaxRetries int               `json:"max_retries"`
	Timeout    time.Duration     `json:"timeout"`
	Headers    map[string]string `json:"headers,omitempty"`
	Active     bool              `json:"active"`
}

// WebhookPayload represents the message envelope dispatched to consumers.
type WebhookPayload struct {
	EventID    string         `json:"event_id"`
	EventType  string         `json:"event_type"`
	WorkflowID string         `json:"workflow_id"`
	RunID      string         `json:"run_id"`
	Timestamp  time.Time      `json:"timestamp"`
	Data       map[string]any `json:"data"`
}

// DeliveryAttempt logs an individual HTTP POST transmission result.
type DeliveryAttempt struct {
	AttemptNumber int           `json:"attempt_number"`
	Timestamp     time.Time     `json:"timestamp"`
	StatusCode    int           `json:"status_code"`
	Duration      time.Duration `json:"duration"`
	Error         string        `json:"error,omitempty"`
}

// DeliveryRecord tracks the full lifecycle and audit trail of a webhook delivery.
type DeliveryRecord struct {
	ID          string            `json:"id"`
	EndpointID  string            `json:"endpoint_id"`
	Payload     WebhookPayload    `json:"payload"`
	Status      DeliveryStatus    `json:"status"`
	Attempts    []DeliveryAttempt `json:"attempts"`
	NextRetryAt *time.Time        `json:"next_retry_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

// EndpointRegistry manages webhook subscriptions and credentials.
type EndpointRegistry struct {
	mu        sync.RWMutex
	endpoints map[string]EndpointConfig
}

// NewRegistry creates an endpoint registry.
func NewRegistry() *EndpointRegistry {
	return &EndpointRegistry{
		endpoints: make(map[string]EndpointConfig),
	}
}

// Register adds or updates a webhook endpoint configuration.
func (r *EndpointRegistry) Register(cfg EndpointConfig) error {
	u, err := url.ParseRequestURI(cfg.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: %s", ErrInvalidURL, cfg.URL)
	}

	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints[cfg.ID] = cfg
	return nil
}

// Get returns the configuration for an endpoint ID.
func (r *EndpointRegistry) Get(id string) (EndpointConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ep, exists := r.endpoints[id]
	if !exists {
		return EndpointConfig{}, fmt.Errorf("%w: %s", ErrEndpointNotFound, id)
	}
	return ep, nil
}

// SubscribedEndpoints returns all active endpoints interested in the given event type.
func (r *EndpointRegistry) SubscribedEndpoints(eventType string) []EndpointConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []EndpointConfig
	for _, ep := range r.endpoints {
		if !ep.Active {
			continue
		}
		if len(ep.Events) == 0 {
			matched = append(matched, ep)
			continue
		}
		for _, pattern := range ep.Events {
			if pattern == "*" || pattern == eventType {
				matched = append(matched, ep)
				break
			}
		}
	}
	return matched
}

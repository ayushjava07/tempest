package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Severity level of notification events.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityError    Severity = "ERROR"
	SeverityCritical Severity = "CRITICAL"
)

// Event describes a workflow lifecycle event for alerting.
type Event struct {
	ID         string            `json:"id"`
	WorkflowID string            `json:"workflow_id"`
	RunID      string            `json:"run_id"`
	EventType  string            `json:"event_type"` // STARTED, SUCCEEDED, FAILED, APPROVAL_PENDING
	Severity   Severity          `json:"severity"`
	Title      string            `json:"title"`
	Message    string            `json:"message"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Channel represents an alert delivery medium (e.g. Webhook, Slack, PagerDuty).
type Channel interface {
	Name() string
	Send(ctx context.Context, ev Event) error
}

// WebhookConfig configures HMAC-signed HTTP webhook alerts.
type WebhookConfig struct {
	URL        string
	SecretKey  string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// WebhookChannel posts signed JSON payloads to remote endpoints.
type WebhookChannel struct {
	cfg WebhookConfig
}

// NewWebhookChannel creates a new Webhook alert channel.
func NewWebhookChannel(cfg WebhookConfig) *WebhookChannel {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &WebhookChannel{cfg: cfg}
}

func (w *WebhookChannel) Name() string {
	return "webhook"
}

func (w *WebhookChannel) Send(ctx context.Context, ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("webhook: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook: create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Tempest-Notifier/1.0")

	// Calculate HMAC signature
	if w.cfg.SecretKey != "" {
		mac := hmac.New(sha256.New, []byte(w.cfg.SecretKey))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Tempest-Signature-256", sig)
	}

	resp, err := w.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: dispatch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}

	return nil
}

// SlackBlockPayload represents simplified Slack Block Kit structure.
type SlackBlockPayload struct {
	Text        string            `json:"text"`
	Attachments []SlackAttachment `json:"attachments"`
}

type SlackAttachment struct {
	Color  string       `json:"color"`
	Title  string       `json:"title"`
	Text   string       `json:"text"`
	Fields []SlackField `json:"fields"`
	Ts     int64        `json:"ts"`
}

type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackChannel formats alerts into Slack Block Kit attachments.
type SlackChannel struct {
	webhookURL string
	client     *http.Client
}

// NewSlackChannel creates a Slack incoming webhook dispatcher.
func NewSlackChannel(webhookURL string, client *http.Client) *SlackChannel {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &SlackChannel{
		webhookURL: webhookURL,
		client:     client,
	}
}

func (s *SlackChannel) Name() string {
	return "slack"
}

func (s *SlackChannel) Send(ctx context.Context, ev Event) error {
	color := "#36a64f" // green
	switch ev.Severity {
	case SeverityWarning:
		color = "#ecaa38" // amber
	case SeverityError:
		color = "#de425b" // red
	case SeverityCritical:
		color = "#8b0000" // dark red
	}

	fields := []SlackField{
		{Title: "Workflow", Value: ev.WorkflowID, Short: true},
		{Title: "Run ID", Value: ev.RunID, Short: true},
		{Title: "Event", Value: ev.EventType, Short: true},
		{Title: "Severity", Value: string(ev.Severity), Short: true},
	}
	for k, v := range ev.Metadata {
		fields = append(fields, SlackField{Title: k, Value: v, Short: true})
	}

	slackMsg := SlackBlockPayload{
		Text: fmt.Sprintf("[%s] %s: %s", ev.Severity, ev.WorkflowID, ev.Title),
		Attachments: []SlackAttachment{
			{
				Color:  color,
				Title:  ev.Title,
				Text:   ev.Message,
				Fields: fields,
				Ts:     ev.Timestamp.Unix(),
			},
		},
	}

	payload, err := json.Marshal(slackMsg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack: received status %d", resp.StatusCode)
	}

	return nil
}

// PagerDutyEvent represents PagerDuty Events API v2 payload.
type PagerDutyEvent struct {
	RoutingKey  string           `json:"routing_key"`
	EventAction string           `json:"event_action"` // trigger, acknowledge, resolve
	DedupKey    string           `json:"dedup_key"`
	Payload     PagerDutyPayload `json:"payload"`
}

type PagerDutyPayload struct {
	Summary       string            `json:"summary"`
	Severity      string            `json:"severity"` // info, warning, error, critical
	Source        string            `json:"source"`
	Timestamp     string            `json:"timestamp"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

// PagerDutyChannel delivers critical incidents to PagerDuty.
type PagerDutyChannel struct {
	routingKey string
	apiURL     string
	client     *http.Client
}

// NewPagerDutyChannel creates a PagerDuty v2 events dispatcher.
func NewPagerDutyChannel(routingKey string, client *http.Client) *PagerDutyChannel {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &PagerDutyChannel{
		routingKey: routingKey,
		apiURL:     "https://events.pagerduty.com/v2/enqueue",
		client:     client,
	}
}

func (p *PagerDutyChannel) Name() string {
	return "pagerduty"
}

func (p *PagerDutyChannel) Send(ctx context.Context, ev Event) error {
	action := "trigger"
	if ev.EventType == "SUCCEEDED" {
		action = "resolve"
	}

	pdSeverity := "error"
	switch ev.Severity {
	case SeverityInfo:
		pdSeverity = "info"
	case SeverityWarning:
		pdSeverity = "warning"
	case SeverityCritical:
		pdSeverity = "critical"
	}

	pdMsg := PagerDutyEvent{
		RoutingKey:  p.routingKey,
		EventAction: action,
		DedupKey:    fmt.Sprintf("%s-%s", ev.WorkflowID, ev.EventType),
		Payload: PagerDutyPayload{
			Summary:       fmt.Sprintf("[%s] %s: %s", ev.Severity, ev.WorkflowID, ev.Title),
			Severity:      pdSeverity,
			Source:        "tempest-orchestrator",
			Timestamp:     ev.Timestamp.Format(time.RFC3339),
			CustomDetails: ev.Metadata,
		},
	}

	payload, err := json.Marshal(pdMsg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pagerduty: received status %d", resp.StatusCode)
	}

	return nil
}

// AntiStormConfig configures alert suppression during cascading failures.
type AntiStormConfig struct {
	CooldownWindow time.Duration
	MaxBurst       int
}

type stormTracker struct {
	firstSeen time.Time
	lastSeen  time.Time
	count     int
}

// Dispatcher orchestrates multi-channel event delivery with storm defense.
type Dispatcher struct {
	mu       sync.Mutex
	channels []Channel
	stormCfg AntiStormConfig
	storms   map[string]*stormTracker
}

// NewDispatcher creates a new notification dispatcher.
func NewDispatcher(stormCfg AntiStormConfig) *Dispatcher {
	return &Dispatcher{
		channels: make([]Channel, 0),
		stormCfg: stormCfg,
		storms:   make(map[string]*stormTracker),
	}
}

// RegisterChannel binds an alert channel to the dispatcher.
func (d *Dispatcher) RegisterChannel(c Channel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels = append(d.channels, c)
}

// Dispatch evaluates storm rate limits and broadcasts the event across channels.
func (d *Dispatcher) Dispatch(ctx context.Context, ev Event) error {
	d.mu.Lock()
	stormKey := fmt.Sprintf("%s:%s", ev.WorkflowID, ev.EventType)
	tracker, ok := d.storms[stormKey]
	now := time.Now()

	if !ok || now.Sub(tracker.lastSeen) > d.stormCfg.CooldownWindow {
		// New window
		d.storms[stormKey] = &stormTracker{
			firstSeen: now,
			lastSeen:  now,
			count:     1,
		}
	} else {
		// Existing active window
		tracker.count++
		tracker.lastSeen = now
		if tracker.count > d.stormCfg.MaxBurst {
			// Suppress alert storm
			d.mu.Unlock()
			return nil
		}
	}

	channelsCopy := make([]Channel, len(d.channels))
	copy(channelsCopy, d.channels)
	d.mu.Unlock()

	var errs []error
	for _, ch := range channelsCopy {
		if err := ch.Send(ctx, ev); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", ch.Name(), err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

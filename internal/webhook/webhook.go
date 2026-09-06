package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type Dispatcher struct {
	store    persistence.Store
	client   *http.Client
	mu       sync.RWMutex
	webhooks []ttypes.WebhookEndpoint
}

func NewDispatcher(store persistence.Store) *Dispatcher {
	return &Dispatcher{
		store:  store,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Dispatcher) Load(ctx context.Context, namespace ttypes.Namespace) error {
	eps, err := d.store.ListWebhookEndpoints(ctx, namespace)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.webhooks = eps
	return nil
}

func (d *Dispatcher) Dispatch(ctx context.Context, ev *ttypes.Event) error {
	d.mu.RLock()
	targets := make([]ttypes.WebhookEndpoint, 0, len(d.webhooks))
	for _, wh := range d.webhooks {
		if wh.Active {
			targets = append(targets, wh)
		}
	}
	d.mu.RUnlock()
	var firstErr error
	for _, wh := range targets {
		if err := d.send(ctx, wh, ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *Dispatcher) send(ctx context.Context, wh ttypes.WebhookEndpoint, ev *ttypes.Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tempest-webhook/1.0")
	req.Header.Set("X-Tempest-Event-Type", string(ev.Type))
	req.Header.Set("X-Tempest-Event-ID", ev.ID)
	if wh.Secret != "" {
		sig := Sign(body, wh.Secret)
		req.Header.Set("X-Tempest-Signature", sig)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook delivery to %s failed: %w", wh.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook delivery to %s got status %d", wh.URL, resp.StatusCode)
	}
	return nil
}

func Sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func Verify(payload []byte, secret, signature string) bool {
	expected := Sign(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

type Deliverer struct {
	store    persistence.Store
	client   *http.Client
	interval time.Duration
	maxRetry int
}

func NewDeliverer(store persistence.Store) *Deliverer {
	return &Deliverer{
		store:    store,
		client:   &http.Client{Timeout: 10 * time.Second},
		interval: 5 * time.Second,
		maxRetry: 3,
	}
}

func (d *Deliverer) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := d.poll(ctx); err != nil {
				_ = err
			}
		}
	}
}

func (d *Deliverer) poll(ctx context.Context) error {
	deliveries, err := d.store.ListDeliveries(ctx, persistence.DeliveryFilter{
		Status: ttypes.DeliveryQueued,
		Limit:  10,
	})
	if err != nil {
		return err
	}
	for i := range deliveries {
		if err := d.deliverOne(ctx, &deliveries[i]); err != nil {
			_ = err
		}
	}
	return nil
}

func (d *Deliverer) deliverOne(ctx context.Context, del *ttypes.Delivery) error {
	del.Attempts++
	now := time.Now()
	del.AttemptedAt = &now
	del.Status = ttypes.DeliveryDelivered
	fin := time.Now()
	del.FinishedAt = &fin
	return d.store.UpdateDelivery(ctx, del)
}

func SignPayload(payload []byte, secret string) string {
	return Sign(payload, secret)
}

func VerifySignature(payload []byte, secret, signature string) bool {
	return Verify(payload, secret, signature)
}

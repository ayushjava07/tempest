package webhook

import (
	"testing"

	"github.com/tempest-io/tempest/internal/persistence/memstore"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestSignAndVerify(t *testing.T) {
	payload := []byte(`{"type":"run.created"}`)
	secret := "my-secret"
	sig := Sign(payload, secret)
	if sig == "" {
		t.Error("expected non-empty signature")
	}
	if !Verify(payload, secret, sig) {
		t.Error("expected Verify to return true")
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	payload := []byte(`{"type":"run.created"}`)
	sig := Sign(payload, "correct-secret")
	if Verify(payload, "wrong-secret", sig) {
		t.Error("expected Verify to return false with wrong secret")
	}
}

func TestVerify_WrongPayload(t *testing.T) {
	sig := Sign([]byte("original"), "secret")
	if Verify([]byte("tampered"), "secret", sig) {
		t.Error("expected Verify to return false with tampered payload")
	}
}

func TestDispatcher_Load(t *testing.T) {
	store := memstore.New()
	d := NewDispatcher(store)
	d.mu.Lock()
	d.webhooks = append(d.webhooks, ttypes.WebhookEndpoint{
		ID: "wh1", URL: "https://example.com", Active: true,
	})
	d.mu.Unlock()
	if len(d.webhooks) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(d.webhooks))
	}
}

func TestDeliverer_Creation(t *testing.T) {
	store := memstore.New()
	d := NewDeliverer(store)
	if d == nil {
		t.Error("expected non-nil deliverer")
	}
}

package webhookdelivery

import (
	"context"
	"testing"
	"time"
)

func TestDeadLetterStoreAndManualRedelivery(t *testing.T) {
	ctx := context.Background()
	dlq := NewDeadLetterStore()

	// Initial failing client
	mockClient := &mockHTTPPoster{
		statusCode: 500,
	}
	worker := NewWorker(mockClient, DefaultBackoff())

	ep := EndpointConfig{
		ID:         "ep-partner",
		URL:        "https://partner.com/events",
		MaxRetries: 2,
		Active:     true,
	}

	record := DeliveryRecord{
		ID:         "deliv-dead-01",
		EndpointID: ep.ID,
		Payload: WebhookPayload{
			EventID: "evt-01",
		},
		Status:    StatusDeadLetter,
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
	}

	dlq.Save(record)
	if dlq.Count() != 1 {
		t.Fatalf("expected 1 record in DLQ, got %d", dlq.Count())
	}

	// 1. Redelivery while downstream still fails
	err := dlq.Redeliver(ctx, "deliv-dead-01", worker, ep)
	if err == nil {
		t.Fatalf("expected redelivery to fail while downstream returns 500")
	}
	if dlq.Count() != 1 {
		t.Fatalf("record should remain in DLQ after failed redelivery")
	}

	// 2. Downstream endpoint recovers: returns 200 OK
	mockClient.mu.Lock()
	mockClient.statusCode = 200
	mockClient.mu.Unlock()

	err = dlq.Redeliver(ctx, "deliv-dead-01", worker, ep)
	if err != nil {
		t.Fatalf("expected successful redelivery once endpoint recovered: %v", err)
	}

	// Successful redelivery purges record from DLQ
	if dlq.Count() != 0 {
		t.Fatalf("expected DLQ empty after successful redelivery, got %d", dlq.Count())
	}
}

func TestDeadLetterPurge(t *testing.T) {
	dlq := NewDeadLetterStore()

	now := time.Now().UTC()
	// Record 1: 5 days old
	dlq.Save(DeliveryRecord{
		ID:        "old-1",
		CreatedAt: now.Add(-5 * 24 * time.Hour),
	})
	// Record 2: 3 days old
	dlq.Save(DeliveryRecord{
		ID:        "old-2",
		CreatedAt: now.Add(-3 * 24 * time.Hour),
	})
	// Record 3: 1 hour old
	dlq.Save(DeliveryRecord{
		ID:        "fresh-3",
		CreatedAt: now.Add(-1 * time.Hour),
	})

	if dlq.Count() != 3 {
		t.Fatalf("expected 3 records")
	}

	// Purge records older than 2 days
	cutoff := now.Add(-2 * 24 * time.Hour)
	purged := dlq.Purge(cutoff)
	if purged != 2 {
		t.Fatalf("expected 2 purged records, got %d", purged)
	}

	if dlq.Count() != 1 {
		t.Fatalf("expected 1 record remaining, got %d", dlq.Count())
	}

	_, exists := dlq.Get("fresh-3")
	if !exists {
		t.Fatalf("expected fresh-3 to be preserved")
	}
}

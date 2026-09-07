package webhookdelivery

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchDispatcherParallelExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dispatched atomic.Uint64
	mockClient := &mockHTTPPoster{
		statusCode: 200,
	}

	worker := NewWorker(mockClient, DefaultBackoff())
	dispatcher := NewBatchDispatcher(worker, 10)

	ep := EndpointConfig{
		ID:         "batch-ep",
		URL:        "https://receiver.internal/hooks",
		MaxRetries: 3,
		Active:     true,
	}
	eps := map[string]EndpointConfig{"batch-ep": ep}

	const totalRecords = 50
	records := make([]*DeliveryRecord, totalRecords)
	for i := 0; i < totalRecords; i++ {
		records[i] = &DeliveryRecord{
			ID:         fmt.Sprintf("rec-%d", i),
			EndpointID: "batch-ep",
			Payload: WebhookPayload{
				EventID: fmt.Sprintf("evt-%d", i),
			},
			Status:    StatusPending,
			CreatedAt: time.Now().UTC(),
		}
	}

	err := dispatcher.DispatchBatch(ctx, records, eps)
	if err != nil {
		t.Fatalf("batch dispatch failed: %v", err)
	}

	for i, r := range records {
		if r.Status != StatusSuccess {
			t.Fatalf("record %d status expected StatusSuccess, got %s", i, r.Status)
		}
		dispatched.Add(1)
	}

	if dispatched.Load() != totalRecords {
		t.Fatalf("expected %d dispatched records, got %d", totalRecords, dispatched.Load())
	}
}

func TestPooledClientCreation(t *testing.T) {
	client := NewPooledHTTPClient(DefaultPooledConfig())
	if client == nil || client.Transport == nil {
		t.Fatalf("failed to initialize pooled HTTP client")
	}
}

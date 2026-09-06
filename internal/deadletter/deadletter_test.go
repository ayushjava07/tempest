package deadletter

import (
	"bytes"
	"testing"
	"time"
)

func TestQueue_EnqueueAndGet(t *testing.T) {
	q := New()

	payload := []byte("error payload")
	rec := q.Enqueue("run-101", "step-A", "connection timeout", 3, payload)

	if rec == nil || rec.ID == "" {
		t.Fatalf("expected non-empty dead letter record")
	}

	fetched, err := q.Get(rec.ID)
	if err != nil {
		t.Fatalf("failed to get record: %v", err)
	}

	if fetched.RunID != "run-101" || fetched.Attempts != 3 || !bytes.Equal(fetched.Payload, payload) {
		t.Errorf("record mismatch: %+v", fetched)
	}
}

func TestQueue_ListAndRedrive(t *testing.T) {
	q := New()
	rec := q.Enqueue("run-102", "step-B", "fatal error", 5, nil)

	list := q.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 record, got %d", len(list))
	}

	if rec.RedrivenAt != nil {
		t.Errorf("expected RedrivenAt to be nil initially")
	}

	if err := q.MarkRedriven(rec.ID); err != nil {
		t.Fatalf("failed to mark redriven: %v", err)
	}

	fetched, _ := q.Get(rec.ID)
	if fetched.RedrivenAt == nil {
		t.Errorf("expected RedrivenAt to be populated after redrive")
	}
}

func TestQueue_Purge(t *testing.T) {
	q := New()
	_ = q.Enqueue("run-103", "step-C", "err", 1, nil)

	purged := q.Purge(time.Hour)
	if purged != 0 {
		t.Errorf("fresh records should not be purged")
	}

	// Purge records older than 0 duration
	purged = q.Purge(0)
	if purged != 1 {
		t.Errorf("expected 1 purged record, got %d", purged)
	}

	if len(q.List()) != 0 {
		t.Errorf("expected empty list after purge")
	}
}

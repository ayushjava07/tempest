package signal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestSignal_SendAndReceive_Buffered(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	sig := &Signal{
		ID:        "sig-1",
		Namespace: "default",
		RunID:     "run-123",
		Name:      "approval",
		Payload:   []byte(`{"approved":true}`),
	}

	// Send before waiter arrives
	if err := mgr.Send(ctx, sig); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if count := mgr.PendingCount("default", "run-123", "approval"); count != 1 {
		t.Fatalf("expected 1 buffered signal, got %d", count)
	}

	// Now receive
	received, err := mgr.Receive(ctx, "default", "run-123", "approval", 1*time.Second, nil)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if received.ID != "sig-1" || !bytes.Equal(received.Payload, sig.Payload) {
		t.Errorf("unexpected received signal: %v", received)
	}

	if count := mgr.PendingCount("default", "run-123", "approval"); count != 0 {
		t.Errorf("expected 0 buffered signals after consumption, got %d", count)
	}
}

func TestSignal_SendAndReceive_ActiveWaiter(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	receivedCh := make(chan *Signal, 1)
	errCh := make(chan error, 1)

	// Launch waiter goroutine first
	go func() {
		sig, err := mgr.Receive(ctx, "default", "run-active", "payment_confirmed", 2*time.Second, nil)
		if err != nil {
			errCh <- err
			return
		}
		receivedCh <- sig
	}()

	// Allow goroutine to register waiter
	time.Sleep(20 * time.Millisecond)

	// Send signal to active waiter
	sig := &Signal{
		ID:        "sig-pay",
		Namespace: "default",
		RunID:     "run-active",
		Name:      "payment_confirmed",
		Payload:   []byte(`{"amount":100}`),
	}
	if err := mgr.Send(ctx, sig); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("Receive failed: %v", err)
	case rec := <-receivedCh:
		if rec.ID != "sig-pay" {
			t.Errorf("unexpected signal ID: %s", rec.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal delivery")
	}
}

func TestSignal_Timeout(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	_, err := mgr.Receive(ctx, "default", "run-timeout", "missing", 50*time.Millisecond, nil)
	if !errors.Is(err, ErrSignalTimeout) {
		t.Fatalf("expected ErrSignalTimeout, got %v", err)
	}
}

func TestSignal_Predicate(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Send an unapproved signal
	_ = mgr.Send(ctx, &Signal{
		ID:        "sig-reject",
		Namespace: "default",
		RunID:     "run-pred",
		Name:      "decision",
		Payload:   []byte("rejected"),
	})

	// Send an approved signal
	_ = mgr.Send(ctx, &Signal{
		ID:        "sig-approve",
		Namespace: "default",
		RunID:     "run-pred",
		Name:      "decision",
		Payload:   []byte("approved"),
	})

	// Wait only for "approved" signal
	received, err := mgr.Receive(ctx, "default", "run-pred", "decision", 1*time.Second, func(s *Signal) bool {
		return string(s.Payload) == "approved"
	})
	if err != nil {
		t.Fatalf("Receive with predicate failed: %v", err)
	}

	if received.ID != "sig-approve" {
		t.Errorf("expected sig-approve, got %s", received.ID)
	}
}

func TestSignal_Concurrency(t *testing.T) {
	mgr := NewManager()
	concurrency := 10

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runID := fmt.Sprintf("run-concurrent-%d", id)
			name := "sync_step"

			go func() {
				time.Sleep(10 * time.Millisecond)
				_ = mgr.Send(context.Background(), &Signal{
					ID:        fmt.Sprintf("sig-%d", id),
					Namespace: "default",
					RunID:     runID,
					Name:      name,
				})
			}()

			rec, err := mgr.Receive(context.Background(), "default", runID, name, 2*time.Second, nil)
			if err != nil || rec == nil {
				t.Errorf("worker %d failed to receive signal: %v", id, err)
			}
		}(i)
	}
	wg.Wait()
}

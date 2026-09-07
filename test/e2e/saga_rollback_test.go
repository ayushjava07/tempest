package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/saga"
)

// ServiceState tracks state for mock distributed services
type ServiceState struct {
	mu           sync.Mutex
	flightBooked bool
	hotelBooked  bool
	carRented    bool
	paymentPaid  bool
	events       []string
}

func (s *ServiceState) record(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *ServiceState) getEvents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]string, len(s.events))
	copy(copied, s.events)
	return copied
}

func TestE2E_MultiStepSagaFailureAndRollback(t *testing.T) {
	orch := saga.NewOrchestrator()
	state := &ServiceState{}

	s := orch.Create("trip-booking-saga-101")

	// Step 1: Flight Reservation
	s.AddStep(&saga.Step{
		Name: "ReserveFlight",
		Execute: func(ctx context.Context) error {
			state.mu.Lock()
			state.flightBooked = true
			state.mu.Unlock()
			state.record("flight:reserved")
			return nil
		},
		Compensate: func(ctx context.Context) error {
			state.mu.Lock()
			state.flightBooked = false
			state.mu.Unlock()
			state.record("flight:cancelled")
			return nil
		},
	})

	// Step 2: Hotel Reservation
	s.AddStep(&saga.Step{
		Name: "BookHotel",
		Execute: func(ctx context.Context) error {
			state.mu.Lock()
			state.hotelBooked = true
			state.mu.Unlock()
			state.record("hotel:booked")
			return nil
		},
		Compensate: func(ctx context.Context) error {
			state.mu.Lock()
			state.hotelBooked = false
			state.mu.Unlock()
			state.record("hotel:refunded")
			return nil
		},
	})

	// Step 3: Car Rental
	s.AddStep(&saga.Step{
		Name: "RentCar",
		Execute: func(ctx context.Context) error {
			state.mu.Lock()
			state.carRented = true
			state.mu.Unlock()
			state.record("car:rented")
			return nil
		},
		Compensate: func(ctx context.Context) error {
			state.mu.Lock()
			state.carRented = false
			state.mu.Unlock()
			state.record("car:released")
			return nil
		},
	})

	// Step 4: Payment Processing (Simulated failure: insufficient credit)
	errPaymentDeclined := errors.New("payment_processor: insufficient credit")
	s.AddStep(&saga.Step{
		Name: "ProcessPayment",
		Execute: func(ctx context.Context) error {
			state.record("payment:declined")
			return errPaymentDeclined
		},
		Compensate: func(ctx context.Context) error {
			// Payment was never completed, compensation should not execute
			state.record("payment:compensate_unexpected")
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.Execute(ctx)
	if err == nil {
		t.Fatalf("expected saga execution to fail with payment error, got nil")
	}
	if !errors.Is(err, errPaymentDeclined) {
		t.Fatalf("expected error %v, got %v", errPaymentDeclined, err)
	}

	if !s.IsFailed() {
		t.Fatalf("expected saga.IsFailed() to be true")
	}
	if s.IsCompleted() {
		t.Fatalf("expected saga.IsCompleted() to be false")
	}

	// Verify executed count: only 3 succeeded before failure
	if s.StepsExecuted() != 3 {
		t.Fatalf("expected 3 successfully executed steps before failure, got %d", s.StepsExecuted())
	}

	// Verify all state changes have been compensated
	state.mu.Lock()
	if state.flightBooked {
		t.Errorf("expected flight booking to be rolled back")
	}
	if state.hotelBooked {
		t.Errorf("expected hotel booking to be rolled back")
	}
	if state.carRented {
		t.Errorf("expected car rental to be rolled back")
	}
	state.mu.Unlock()

	// Verify LIFO compensation order:
	// Execution order: flight:reserved -> hotel:booked -> car:rented -> payment:declined
	// Rollback order: car:released -> hotel:refunded -> flight:cancelled
	expectedEvents := []string{
		"flight:reserved",
		"hotel:booked",
		"car:rented",
		"payment:declined",
		"car:released",
		"hotel:refunded",
		"flight:cancelled",
	}

	events := state.getEvents()
	if len(events) != len(expectedEvents) {
		t.Fatalf("unexpected event log length: want %d, got %d: %v", len(expectedEvents), len(events), events)
	}
	for i, want := range expectedEvents {
		if events[i] != want {
			t.Errorf("event [%d]: expected %q, got %q", i, want, events[i])
		}
	}
}

func TestE2E_ConcurrentSagasIsolation(t *testing.T) {
	orch := saga.NewOrchestrator()
	const numSagas = 20
	var wg sync.WaitGroup
	wg.Add(numSagas)

	for i := 0; i < numSagas; i++ {
		sagaID := fmt.Sprintf("concurrent-saga-%03d", i)
		shouldFail := (i%2 == 0)

		go func(id string, fail bool) {
			defer wg.Done()
			s := orch.Create(id)
			compensated := false

			s.AddStep(&saga.Step{
				Name: "Reserve",
				Execute: func(ctx context.Context) error {
					return nil
				},
				Compensate: func(ctx context.Context) error {
					compensated = true
					return nil
				},
			})

			s.AddStep(&saga.Step{
				Name: "Finalize",
				Execute: func(ctx context.Context) error {
					if fail {
						return errors.New("simulated error")
					}
					return nil
				},
			})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := s.Execute(ctx)
			if fail && (err == nil || !compensated) {
				t.Errorf("saga %s expected failure and rollback, err=%v compensated=%v", id, err, compensated)
			} else if !fail && (err != nil || compensated) {
				t.Errorf("saga %s expected success, err=%v compensated=%v", id, err, compensated)
			}
		}(sagaID, shouldFail)
	}

	wg.Wait()

	if len(orch.List()) != numSagas {
		t.Fatalf("expected %d sagas registered in orchestrator, got %d", numSagas, len(orch.List()))
	}
}

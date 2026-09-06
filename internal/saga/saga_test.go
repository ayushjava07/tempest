package saga

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestSaga_SuccessfulExecution(t *testing.T) {
	coord := NewCoordinator("saga-success")
	ctx := context.Background()

	var executed []string
	var mu sync.Mutex

	_ = coord.AddStep("step-1", func(ctx context.Context) error {
		mu.Lock()
		executed = append(executed, "1")
		mu.Unlock()
		return nil
	}, nil)

	_ = coord.AddStep("step-2", func(ctx context.Context) error {
		mu.Lock()
		executed = append(executed, "2")
		mu.Unlock()
		return nil
	}, nil)

	err := coord.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if coord.State() != StateCompleted {
		t.Errorf("expected StateCompleted, got %v", coord.State())
	}
	if len(executed) != 2 || executed[0] != "1" || executed[1] != "2" {
		t.Errorf("unexpected executed order: %v", executed)
	}
}

func TestSaga_CompensateOnFailure(t *testing.T) {
	coord := NewCoordinator("saga-compensate")
	ctx := context.Background()

	var compensated []string
	var mu sync.Mutex

	// Step 1
	_ = coord.AddStep("reserve_inventory",
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error {
			mu.Lock()
			compensated = append(compensated, "release_inventory")
			mu.Unlock()
			return nil
		},
	)

	// Step 2
	_ = coord.AddStep("authorize_payment",
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error {
			mu.Lock()
			compensated = append(compensated, "void_payment")
			mu.Unlock()
			return nil
		},
	)

	// Step 3 (Fails)
	_ = coord.AddStep("ship_goods",
		func(ctx context.Context) error {
			return errors.New("warehouse out of stock")
		},
		func(ctx context.Context) error {
			mu.Lock()
			compensated = append(compensated, "cancel_shipping")
			mu.Unlock()
			return nil
		},
	)

	err := coord.Execute(ctx)
	if !errors.Is(err, ErrSagaAborted) {
		t.Fatalf("expected ErrSagaAborted, got %v", err)
	}

	if coord.State() != StateCompensated {
		t.Errorf("expected StateCompensated, got %v", coord.State())
	}

	// Compensations must run in reverse order (authorize_payment then reserve_inventory)
	if len(compensated) != 2 {
		t.Fatalf("expected 2 compensations, got %d (%v)", len(compensated), compensated)
	}
	if compensated[0] != "void_payment" || compensated[1] != "release_inventory" {
		t.Errorf("expected [void_payment, release_inventory], got %v", compensated)
	}
}

func TestSaga_PartialCompensationOnFailure(t *testing.T) {
	coord := NewCoordinator("saga-partial")
	ctx := context.Background()

	// Step 1: compensation fails
	_ = coord.AddStep("s1",
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return errors.New("gateway unreachable") },
	)

	// Step 2: forward fails
	_ = coord.AddStep("s2",
		func(ctx context.Context) error { return errors.New("fatal s2 failure") },
		nil,
	)

	err := coord.Execute(ctx)
	if !errors.Is(err, ErrCompensationFailed) {
		t.Fatalf("expected ErrCompensationFailed, got %v", err)
	}

	if coord.State() != StatePartiallyCompensated {
		t.Errorf("expected StatePartiallyCompensated, got %v", coord.State())
	}
}

func TestSaga_DuplicateStepID(t *testing.T) {
	coord := NewCoordinator("saga-dup")
	_ = coord.AddStep("same", func(ctx context.Context) error { return nil }, nil)
	err := coord.AddStep("same", func(ctx context.Context) error { return nil }, nil)
	if !errors.Is(err, ErrStepAlreadyRegistered) {
		t.Errorf("expected ErrStepAlreadyRegistered, got %v", err)
	}
}

func TestSaga_Concurrency(t *testing.T) {
	concurrency := 10
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c := NewCoordinator(fmt.Sprintf("saga-%d", id))
			_ = c.AddStep("step-a", func(ctx context.Context) error { return nil }, nil)
			_ = c.AddStep("step-b", func(ctx context.Context) error { return nil }, nil)
			err := c.Execute(context.Background())
			if err != nil || c.State() != StateCompleted {
				t.Errorf("saga %d failed: %v", id, err)
			}
		}(i)
	}
	wg.Wait()
}

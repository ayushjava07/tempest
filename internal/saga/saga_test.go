package saga

import (
	"context"
	"fmt"
	"testing"
)

func TestSaga_Basic(t *testing.T) {
	s := New()
	executed := []string{}
	s.AddStep(&Step{
		Name: "step1",
		Execute: func(ctx context.Context) error {
			executed = append(executed, "step1")
			return nil
		},
		Compensate: func(ctx context.Context) error {
			executed = append(executed, "compensate1")
			return nil
		},
	})
	if err := s.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.IsCompleted() {
		t.Error("expected completed")
	}
	if len(executed) != 1 || executed[0] != "step1" {
		t.Error("expected step1 executed")
	}
}

func TestSaga_Compensation(t *testing.T) {
	s := New()
	executed := []string{}
	s.AddStep(&Step{
		Name: "step1",
		Execute: func(ctx context.Context) error {
			executed = append(executed, "step1")
			return nil
		},
		Compensate: func(ctx context.Context) error {
			executed = append(executed, "compensate1")
			return nil
		},
	})
	s.AddStep(&Step{
		Name: "step2",
		Execute: func(ctx context.Context) error {
			executed = append(executed, "step2")
			return fmt.Errorf("step2 failed")
		},
		Compensate: func(ctx context.Context) error {
			executed = append(executed, "compensate2")
			return nil
		},
	})
	err := s.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !s.IsFailed() {
		t.Error("expected failed")
	}
	expected := []string{"step1", "step2", "compensate1"}
	if len(executed) != len(expected) {
		t.Errorf("expected %v, got %v", expected, executed)
	}
	for i, v := range expected {
		if executed[i] != v {
			t.Errorf("expected %v, got %v", expected, executed)
		}
	}
}

func TestSaga_StepsCount(t *testing.T) {
	s := New()
	s.AddStep(&Step{Name: "a", Execute: func(ctx context.Context) error { return nil }})
	s.AddStep(&Step{Name: "b", Execute: func(ctx context.Context) error { return nil }})
	if s.StepsTotal() != 2 {
		t.Error("expected 2 steps")
	}
	if s.StepsExecuted() != 0 {
		t.Error("expected 0 executed")
	}
	s.Execute(context.Background())
	if s.StepsExecuted() != 2 {
		t.Error("expected 2 executed")
	}
}

func TestSaga_DoubleExecute(t *testing.T) {
	s := New()
	s.AddStep(&Step{Name: "a", Execute: func(ctx context.Context) error { return nil }})
	s.Execute(context.Background())
	err := s.Execute(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestOrchestrator_Basic(t *testing.T) {
	o := NewOrchestrator()
	s := o.Create("saga-1")
	s.AddStep(&Step{Name: "a", Execute: func(ctx context.Context) error { return nil }})
	got, ok := o.Get("saga-1")
	if !ok || got != s {
		t.Error("expected saga")
	}
	if len(o.List()) != 1 {
		t.Error("expected 1 saga")
	}
	o.Delete("saga-1")
	if _, ok := o.Get("saga-1"); ok {
		t.Error("expected deleted")
	}
}
package handlers

import (
	"context"
	"testing"

	"github.com/tempest-io/tempest/internal/plugin"
)

func TestPassHandler(t *testing.T) {
	h := &PassHandler{}
	result, err := h.Execute(context.Background(), plugin.Handle{
		RunInput: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output["ok"] != true {
		t.Errorf("expected ok=true, got %v", result.Output)
	}
}

func TestPassHandler_Fail(t *testing.T) {
	h := &PassHandler{}
	result, err := h.Execute(context.Background(), plugin.Handle{
		RunInput: map[string]any{"fail": true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil {
		t.Error("expected error")
	}
}

func TestEchoHandler(t *testing.T) {
	h := &EchoHandler{}
	result, err := h.Execute(context.Background(), plugin.Handle{
		RunID:   "r1",
		StepID:  "s1",
		RunInput: map[string]any{"foo": "bar"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %v", result.Output)
	}
	if result.Output["run_id"] != "r1" {
		t.Errorf("expected run_id=r1, got %v", result.Output)
	}
}

func TestFailHandler(t *testing.T) {
	h := &FailHandler{}
	result, err := h.Execute(context.Background(), plugin.Handle{
		RunInput: map[string]any{"message": "boom"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil {
		t.Error("expected error")
	}
}

func TestHandlerNames(t *testing.T) {
	handlers := []plugin.Handler{
		&PassHandler{}, &EchoHandler{}, &ShellHandler{}, &HTTPHandler{}, &FailHandler{},
	}
	for _, h := range handlers {
		if h.Name() == "" {
			t.Errorf("handler %T has empty name", h)
		}
		if h.Description() == "" {
			t.Errorf("handler %T has empty description", h)
		}
	}
}

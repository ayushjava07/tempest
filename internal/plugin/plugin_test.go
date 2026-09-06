package plugin

import (
	"context"
	"testing"
)

type stubHandler struct {
	name string
}

func (h *stubHandler) Name() string        { return h.name }
func (h *stubHandler) Description() string { return "stub" }
func (h *stubHandler) Execute(_ context.Context, _ Handle) (Result, error) {
	return Result{Output: map[string]any{"ok": true}}, nil
}

func TestRegistry_RegisterAndResolve(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubHandler{name: "pass"})
	h, err := r.Resolve("pass")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if h.Name() != "pass" {
		t.Errorf("expected pass, got %s", h.Name())
	}
}

func TestRegistry_ResolveNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent handler")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubHandler{name: "a"})
	r.Register(&stubHandler{name: "b"})
	names := r.List()
	if len(names) != 2 {
		t.Errorf("expected 2 handlers, got %d", len(names))
	}
}

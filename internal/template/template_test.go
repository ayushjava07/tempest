package template

import (
	"errors"
	"reflect"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTemplate_BasicRender(t *testing.T) {
	engine := NewEngine()
	ctx := Context{
		RunID: "run-999",
		Input: map[string]any{
			"dataset": "customers",
			"count":   100,
		},
		Env: map[string]string{
			"REGION": "us-east-1",
		},
	}

	tmpl := "Executing {{ run.id }} on {{ input.dataset }} (total {{ input.count }}) in {{ env.REGION }}"
	rendered, err := engine.Render(tmpl, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Executing run-999 on customers (total 100) in us-east-1"
	if rendered != expected {
		t.Errorf("rendered %q != expected %q", rendered, expected)
	}
}

func TestTemplate_NestedAndFilter(t *testing.T) {
	engine := NewEngine()
	ctx := Context{
		Steps: map[string]map[string]any{
			"step-1": {
				"output": map[string]any{
					"status": " success ",
				},
			},
		},
	}

	tmpl := "Result is [{{ steps.step-1.output.status | trim | upper }}]"
	rendered, err := engine.Render(tmpl, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Result is [SUCCESS]"
	if rendered != expected {
		t.Errorf("rendered %q != expected %q", rendered, expected)
	}
}

func TestTemplate_DefaultFilter(t *testing.T) {
	engine := NewEngine()
	ctx := Context{
		Input: map[string]any{},
	}

	tmpl := "Retries: {{ input.missing_retries | default(5) }}"
	rendered, err := engine.Render(tmpl, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Retries: 5"
	if rendered != expected {
		t.Errorf("rendered %q != expected %q", rendered, expected)
	}
}

func TestTemplate_RenderObject(t *testing.T) {
	engine := NewEngine()
	ctx := Context{
		Input: map[string]any{
			"retries": 3,
			"target":  "production",
		},
	}

	obj := map[string]any{
		"env":      "{{ input.target }}",
		"attempts": "{{ input.retries }}",
		"tags":     []any{"tag-{{ input.target }}"},
	}

	rendered, err := engine.RenderObject(obj, ctx)
	if err != nil {
		t.Fatalf("RenderObject failed: %v", err)
	}

	m, ok := rendered.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", rendered)
	}

	if m["env"] != "production" {
		t.Errorf("expected env=production, got %v", m["env"])
	}
	if !reflect.DeepEqual(m["attempts"], 3) {
		t.Errorf("expected typed attempts=3, got %v (%T)", m["attempts"], m["attempts"])
	}
}

func TestTemplate_SyntaxErrors(t *testing.T) {
	engine := NewEngine()
	ctx := Context{}

	// Unclosed delimiter
	_, err := engine.Render("Hello {{ input.name", ctx)
	if !errors.Is(err, ErrUnclosedDelimiter) {
		t.Errorf("expected ErrUnclosedDelimiter, got %v", err)
	}

	// Missing key without default
	_, err = engine.Render("Hello {{ input.unknown }}", ctx)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

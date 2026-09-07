package expression

import (
	"testing"
)

func TestEvaluator_SetGetVariable(t *testing.T) {
	e := New()
	e.SetVariable("x", 42)
	v, ok := e.GetVariable("x")
	if !ok || v != 42 {
		t.Error("expected x=42")
	}
}

func TestEvaluator_VariableReference(t *testing.T) {
	e := New()
	e.SetVariable("name", "alice")
	result, err := e.Evaluate("$name")
	if err != nil {
		t.Fatal(err)
	}
	if result != "alice" {
		t.Errorf("expected alice, got %v", result)
	}
}

func TestEvaluator_VariableNotFound(t *testing.T) {
	e := New()
	_, err := e.Evaluate("$missing")
	if err == nil {
		t.Error("expected error")
	}
}

func TestEvaluator_Integer(t *testing.T) {
	e := New()
	result, err := e.Evaluate("42")
	if err != nil {
		t.Fatal(err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestEvaluator_Float(t *testing.T) {
	e := New()
	result, err := e.Evaluate("3.14")
	if err != nil {
		t.Fatal(err)
	}
	if result != 3.14 {
		t.Errorf("expected 3.14, got %v", result)
	}
}

func TestEvaluator_Boolean(t *testing.T) {
	e := New()
	result, err := e.Evaluate("true")
	if err != nil {
		t.Fatal(err)
	}
	if result != true {
		t.Error("expected true")
	}
	result, err = e.Evaluate("false")
	if err != nil {
		t.Fatal(err)
	}
	if result != false {
		t.Error("expected false")
	}
}

func TestEvaluator_String(t *testing.T) {
	e := New()
	result, err := e.Evaluate(`"hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("expected hello, got %v", result)
	}
}

func TestEvaluator_EvaluateBool(t *testing.T) {
	e := New()
	e.SetVariable("flag", true)
	b, err := e.EvaluateBool("$flag")
	if err != nil || !b {
		t.Error("expected true")
	}
	e.SetVariable("flag", 0)
	b, err = e.EvaluateBool("$flag")
	if err != nil || b {
		t.Error("expected false")
	}
}

func TestEvaluator_EvaluateInt(t *testing.T) {
	e := New()
	e.SetVariable("num", 10)
	i, err := e.EvaluateInt("$num")
	if err != nil || i != 10 {
		t.Error("expected 10")
	}
	e.SetVariable("str", "42")
	i, err = e.EvaluateInt("$str")
	if err != nil || i != 42 {
		t.Error("expected 42 from string")
	}
}
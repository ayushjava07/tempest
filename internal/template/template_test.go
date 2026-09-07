package template

import (
	"testing"
)

func TestEngine_Execute(t *testing.T) {
	e := NewEngine()
	result, err := e.Execute("test", "Hello {{.Name}}!", map[string]string{"Name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Hello World!" {
		t.Errorf("expected Hello World!, got %s", result)
	}
}

func TestEngine_Functions(t *testing.T) {
	e := NewEngine()
	result, err := e.Execute("test", "{{upper .}}", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("expected hello, got %s", result)
	}
}

func TestEngine_Default(t *testing.T) {
	e := NewEngine()
	result, err := e.Execute("test", "{{default \"def\" .Val}}", map[string]string{"Val": "val"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "val" {
		t.Errorf("expected val, got %s", result)
	}
	result, err = e.Execute("test", "{{default \"def\" .Val}}", map[string]string{"Val": ""})
	if err != nil {
		t.Fatal(err)
	}
	if result != "def" {
		t.Errorf("expected def, got %s", result)
	}
}

func TestEngine_AddFunc(t *testing.T) {
	e := NewEngine()
	e.AddFunc("double", func(n int) int { return n * 2 })
	result, err := e.Execute("test", "{{double .}}", 5)
	if err != nil {
		t.Fatal(err)
	}
	if result != "10" {
		t.Errorf("expected 10, got %s", result)
	}
}

func TestLoader_LoadExecute(t *testing.T) {
	l := NewLoader()
	if err := l.Load("greet", "Hello {{.Name}}!"); err != nil {
		t.Fatal(err)
	}
	if !l.Has("greet") {
		t.Error("expected template loaded")
	}
	result, err := l.Execute("greet", map[string]string{"Name": "Test"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Hello Test!" {
		t.Errorf("expected Hello Test!, got %s", result)
	}
}

func TestLoader_Missing(t *testing.T) {
	l := NewLoader()
	_, err := l.Execute("missing", nil)
	if err == nil {
		t.Error("expected error")
	}
}
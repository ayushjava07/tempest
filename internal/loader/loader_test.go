package loader

import (
	"encoding/json"
	"testing"
)

func TestLoader_Load(t *testing.T) {
	src := &MemorySource{Data: []byte(`{"key":"value"}`)}
	l := New(Config{Sources: []Source{src}})
	if err := l.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !l.IsLoaded() {
		t.Error("expected loaded")
	}
}

func TestLoader_Get(t *testing.T) {
	src := &MemorySource{Data: []byte(`{"a":1}`)}
	l := New(Config{Sources: []Source{src}})
	l.Load()
	v := l.Get()
	if v == nil {
		t.Error("expected value")
	}
	m := v.(map[string]interface{})
	if m["a"].(float64) != 1 {
		t.Error("expected a=1")
	}
}

func TestLoader_GetBytes(t *testing.T) {
	src := &MemorySource{Data: []byte(`{"x":2}`)}
	l := New(Config{Sources: []Source{src}})
	l.Load()
	data, err := l.GetBytes()
	if err != nil {
		t.Fatalf("GetBytes: %v", err)
	}
	var m map[string]int
	json.Unmarshal(data, &m)
	if m["x"] != 2 {
		t.Error("expected x=2")
	}
}

func TestLoader_NotLoaded(t *testing.T) {
	l := New(Config{})
	if l.IsLoaded() {
		t.Error("expected not loaded")
	}
	if l.Get() != nil {
		t.Error("expected nil")
	}
}

func TestLoader_Reload(t *testing.T) {
	src := &MemorySource{Data: []byte(`{"v":1}`)}
	l := New(Config{Sources: []Source{src}})
	l.Load()
	l.Reload()
	if !l.IsLoaded() {
		t.Error("expected loaded after reload")
	}
}

func TestLoader_InvalidJSON(t *testing.T) {
	src := &MemorySource{Data: []byte(`not json`)}
	l := New(Config{Sources: []Source{src}})
	if err := l.Load(); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFileSource(t *testing.T) {
	src := &FileSource{Path: "/nonexistent"}
	_, err := src.Load()
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoader_GetBytes_NotLoaded(t *testing.T) {
	l := New(Config{})
	_, err := l.GetBytes()
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoader_EmptySources(t *testing.T) {
	l := New(Config{Sources: []Source{}})
	if err := l.Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

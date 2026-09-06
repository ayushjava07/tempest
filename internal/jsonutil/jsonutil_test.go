package jsonutil

import (
	"strings"
	"testing"
)

func TestMarshalIndent(t *testing.T) {
	data := map[string]int{"a": 1}
	b, err := MarshalIndent(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "  ") {
		t.Error("expected indented")
	}
}

func TestUnmarshal(t *testing.T) {
	data := []byte(`{"x":1}`)
	var m map[string]int
	if err := Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["x"] != 1 {
		t.Error("expected x=1")
	}
}

func TestUnmarshal_UnknownFields(t *testing.T) {
	data := []byte(`{"Name":"alice","Age":30,"Extra":"x"}`)
	type simple struct {
		Name string `json:"Name"`
		Age  int    `json:"Age"`
	}
	var m simple
	if err := Unmarshal(data, &m); err == nil {
		t.Error("expected error for unknown fields")
	}
}

func TestUnmarshalLenient(t *testing.T) {
	data := []byte(`{"a":1,"b":2}`)
	var m map[string]int
	if err := UnmarshalLenient(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["a"] != 1 {
		t.Error("expected a=1")
	}
}

func TestPrettyPrint(t *testing.T) {
	s, err := PrettyPrint(map[string]int{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "x") {
		t.Error("expected formatted output")
	}
}

func TestMinify(t *testing.T) {
	data := []byte(`{ "a" : 1 }`)
	minified, err := Minify(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(minified), " ") {
		t.Error("expected no spaces")
	}
}

func TestValidate(t *testing.T) {
	if err := Validate([]byte(`{"valid":true}`)); err != nil {
		t.Error("expected valid")
	}
	if err := Validate([]byte(`not json`)); err == nil {
		t.Error("expected invalid")
	}
}

func TestMerge(t *testing.T) {
	base := []byte(`{"a":1,"b":2}`)
	override := []byte(`{"b":3,"c":4}`)
	merged, err := Merge(base, override)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]int
	UnmarshalLenient(merged, &m)
	if m["a"] != 1 || m["b"] != 3 || m["c"] != 4 {
		t.Error("unexpected merge result")
	}
}

func TestKeys(t *testing.T) {
	data := []byte(`{"a":1,"b":2}`)
	keys, err := Keys(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2, got %d", len(keys))
	}
}

func TestGet(t *testing.T) {
	data := []byte(`{"a":{"b":42}}`)
	v, err := Get(data, "a.b")
	if err != nil {
		t.Fatal(err)
	}
	if v.(float64) != 42 {
		t.Error("expected 42")
	}
}

func TestGet_NotFound(t *testing.T) {
	data := []byte(`{"a":1}`)
	_, err := Get(data, "missing")
	if err == nil {
		t.Error("expected error")
	}
}

func TestValidJSON(t *testing.T) {
	if !ValidJSON(`{"x":1}`) {
		t.Error("expected valid")
	}
	if ValidJSON(`bad`) {
		t.Error("expected invalid")
	}
}

func TestNormalize(t *testing.T) {
	data := []byte(`{  "x"  :  1  }`)
	normalized, err := Normalize(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(normalized) != `{"x":1}` {
		t.Errorf("got %s", normalized)
	}
}

func TestFromJSON_ToJSON(t *testing.T) {
	type T struct {
		A int `json:"a"`
	}
	t1 := T{A: 1}
	s, _ := ToJSON(t1)
	var t2 T
	FromJSON(s, &t2)
	if t2.A != 1 {
		t.Error("expected roundtrip")
	}
}

package csvutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarshal(t *testing.T) {
	records := [][]string{
		{"name", "age"},
		{"alice", "30"},
		{"bob", "25"},
	}
	data, err := Marshal(records)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), "alice,30") {
		t.Error("expected alice,30")
	}
}

func TestUnmarshal(t *testing.T) {
	data := []byte("a,b\n1,2\n")
	records, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(records) != 2 || records[1][0] != "1" {
		t.Error("unexpected records")
	}
}

type person struct {
	Name string `csv:"name"`
	Age  int    `csv:"age"`
}

func TestMarshalStructs(t *testing.T) {
	items := []person{
		{Name: "alice", Age: 30},
		{Name: "bob", Age: 25},
	}
	records, err := MarshalStructs(items)
	if err != nil {
		t.Fatalf("MarshalStructs: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 rows, got %d", len(records))
	}
	if records[0][0] != "name" {
		t.Error("expected name header")
	}
}

func TestMarshalStructs_Empty(t *testing.T) {
	records, err := MarshalStructs[person](nil)
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Error("expected nil")
	}
}

func TestReadAll(t *testing.T) {
	r := strings.NewReader("x,y\n1,2\n")
	records, err := ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2, got %d", len(records))
	}
}

func TestWriteAll(t *testing.T) {
	var buf bytes.Buffer
	records := [][]string{{"a", "b"}, {"1", "2"}}
	if err := WriteAll(&buf, records); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "a,b") {
		t.Error("expected CSV output")
	}
}

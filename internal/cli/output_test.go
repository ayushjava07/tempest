package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	WriteTable(&buf, []string{"NAME", "STATE"}, [][]string{
		{"wf-1", "RUNNING"},
		{"wf-2", "SUCCEEDED"},
	})
	output := buf.String()
	if !strings.Contains(output, "NAME") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "wf-1") {
		t.Error("expected data in output")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	if err := WriteJSON(&buf, data); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected value, got %s", result["key"])
	}
}

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, []string{"A", "B"}, [][]string{{"1", "2"}, {"3", "4"}}); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "A,B") {
		t.Error("expected header")
	}
	if !strings.Contains(output, "1,2") {
		t.Error("expected data")
	}
}

func TestWriteCSV_Escaping(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, []string{"VAL"}, [][]string{{"has,comma"}}); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, `"has,comma"`) {
		t.Errorf("expected escaped value, got %s", output)
	}
}

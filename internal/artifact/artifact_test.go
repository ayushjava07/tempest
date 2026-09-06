package artifact

import (
	"bytes"
	"strings"
	"testing"
)

func TestStore_PutAndGet(t *testing.T) {
	s := NewStore()

	content := "step output data artifact"
	art, err := s.Put("run-1", "result.json", "application/json", strings.NewReader(content))
	if err != nil {
		t.Fatalf("failed to put artifact: %v", err)
	}

	if art.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), art.Size)
	}

	fetched, err := s.Get(art.ID)
	if err != nil {
		t.Fatalf("failed to get artifact: %v", err)
	}

	if !bytes.Equal(fetched.Data, []byte(content)) {
		t.Errorf("content mismatch: %s != %s", fetched.Data, content)
	}

	_, err = s.Get("unknown")
	if err != ErrArtifactNotFound {
		t.Errorf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestStore_LogsStreaming(t *testing.T) {
	s := NewStore()

	s.AppendLogChunk("run-1", "step-1", []byte("line 1\n"))
	s.AppendLogChunk("run-1", "step-1", []byte("line 2\n"))

	logs := s.GetLogs("run-1", "step-1")
	expected := "line 1\nline 2\n"

	if string(logs) != expected {
		t.Errorf("expected %q, got %q", expected, string(logs))
	}

	emptyLogs := s.GetLogs("run-1", "non-existent")
	if emptyLogs != nil {
		t.Errorf("expected nil for non-existent step logs")
	}
}

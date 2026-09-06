package wal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWAL_AppendAndReplay(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tempest.wal")

	l, err := Open(logPath)
	if err != nil {
		t.Fatalf("failed to open wal: %v", err)
	}

	seq1, err := l.Append(1, []byte("action-1"))
	if err != nil || seq1 != 1 {
		t.Fatalf("unexpected seq1: %d (err: %v)", seq1, err)
	}

	seq2, err := l.Append(2, []byte("action-2"))
	if err != nil || seq2 != 2 {
		t.Fatalf("unexpected seq2: %d (err: %v)", seq2, err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("failed to close wal: %v", err)
	}

	records, err := Replay(logPath)
	if err != nil {
		t.Fatalf("failed to replay wal: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[0].SeqID != 1 || !bytes.Equal(records[0].Data, []byte("action-1")) {
		t.Errorf("record 0 mismatch: %+v", records[0])
	}
	if records[1].SeqID != 2 || !bytes.Equal(records[1].Data, []byte("action-2")) {
		t.Errorf("record 1 mismatch: %+v", records[1])
	}
}

func TestWAL_CorruptChecksum(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "corrupt.wal")

	l, err := Open(logPath)
	if err != nil {
		t.Fatalf("failed to open wal: %v", err)
	}

	_, _ = l.Append(1, []byte("payload"))
	_ = l.Close()

	// Corrupt a byte in the payload
	data, _ := os.ReadFile(logPath)
	data[len(data)-5] ^= 0xFF
	_ = os.WriteFile(logPath, data, 0644)

	_, err = Replay(logPath)
	if err != ErrChecksumMismatch {
		t.Errorf("expected ErrChecksumMismatch, got %v", err)
	}
}

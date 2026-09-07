package wal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWAL_OpenClose(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	w, err := Open(filepath.Join(dir, "wal.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWAL_AppendRead(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	w, err := Open(filepath.Join(dir, "wal.log"), 1)
	if err != nil {
		t.Fatal(err)
	}
	e := Entry{Type: "test", Data: []byte("hello")}
	if err := w.Append(e); err != nil {
		t.Fatal(err)
	}
	entries, err := w.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1, got %d", len(entries))
	}
	if string(entries[0].Data) != "hello" {
		t.Error("data mismatch")
	}
	w.Close()
}

func TestWAL_IndexIncrements(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	w, err := Open(filepath.Join(dir, "wal.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		w.Append(Entry{Type: "t", Data: []byte("d")})
	}
	if w.LastIndex() != 5 {
		t.Errorf("expected 5, got %d", w.LastIndex())
	}
	w.Close()
}

func TestWAL_PersistAcrossReopen(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "wal.log")
	w, _ := Open(path, 10)
	w.Append(Entry{Type: "a", Data: []byte("x")})
	w.Append(Entry{Type: "b", Data: []byte("y")})
	w.Close()
	w2, _ := Open(path, 10)
	entries, _ := w2.ReadAll()
	if len(entries) != 2 {
		t.Errorf("expected 2, got %d", len(entries))
	}
	w2.Close()
}

func TestWAL_Term(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	w, _ := Open(filepath.Join(dir, "wal.log"), 10)
	w.SetTerm(5)
	if w.Term() != 5 {
		t.Error("expected term 5")
	}
	w.Close()
}

func TestWAL_Truncate(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	w, _ := Open(filepath.Join(dir, "wal.log"), 10)
	for i := 0; i < 10; i++ {
		w.Append(Entry{Type: "t", Data: []byte("d")})
	}
	w.Truncate(5)
	if w.LastIndex() != 5 {
		t.Errorf("expected 5, got %d", w.LastIndex())
	}
	w.Close()
}

func TestWAL_EntryTimestamp(t *testing.T) {
	dir, _ := os.MkdirTemp("", "wal-*")
	defer os.RemoveAll(dir)
	w, _ := Open(filepath.Join(dir, "wal.log"), 1)
	w.Append(Entry{Type: "t", Data: []byte("d")})
	entries, _ := w.ReadAll()
	if len(entries) == 0 {
		t.Fatal("expected entry")
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("expected timestamp")
	}
	if entries[0].Timestamp.Before(time.Now().Add(-time.Second)) {
		t.Error("timestamp too old")
	}
	w.Close()
}
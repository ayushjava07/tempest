package sandbox

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSandbox_Run(t *testing.T) {
	s := New(Config{Timeout: 5 * time.Second})
	out, err := s.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Error("expected output")
	}
}

func TestSandbox_Timeout(t *testing.T) {
	s := New(Config{Timeout: 50 * time.Millisecond})
	_, err := s.Run(context.Background(), "sleep", "1")
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestSandbox_Cleanup(t *testing.T) {
	dir, _ := os.MkdirTemp("", "sandbox-test-*")
	s := New(Config{WorkDir: dir, Timeout: time.Second})
	s.Run(context.Background(), "echo", "test")
	if err := s.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("expected dir removed")
	}
}

func TestFileSystem_ReadWrite(t *testing.T) {
	dir, _ := os.MkdirTemp("", "fs-test-*")
	fs := NewFileSystem(dir)
	if err := fs.Write("test.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	data, err := fs.Read("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Error("data mismatch")
	}
}

func TestFileSystem_List(t *testing.T) {
	dir, _ := os.MkdirTemp("", "fs-test-*")
	fs := NewFileSystem(dir)
	fs.Write("a.txt", []byte("a"))
	fs.Write("b.txt", []byte("b"))
	list, err := fs.List(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestFileSystem_AccessDenied(t *testing.T) {
	dir, _ := os.MkdirTemp("", "fs-test-*")
	fs := NewFileSystem(dir)
	_, err := fs.Read("../etc/passwd")
	if err == nil {
		t.Error("expected access denied")
	}
}
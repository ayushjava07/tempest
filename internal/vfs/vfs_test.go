package vfs

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestVFS_PathTraversalDefense(t *testing.T) {
	fs := New("/tenants/tenant-42", DefaultConfig())

	traversals := []string{
		"../escape.txt",
		"../../etc/passwd",
		"/../outside.txt",
		"a/../../outside.txt",
		"/../../outside.txt",
		"nested/../../../root.txt",
		"file\x00traversal.txt",
	}

	for _, p := range traversals {
		t.Run("traverse_"+p, func(t *testing.T) {
			err := fs.WriteFile(p, []byte("forbidden"))
			if err == nil {
				t.Fatalf("expected path traversal error for %q, got nil", p)
			}

			_, err = fs.ReadFile(p)
			if err == nil {
				t.Fatalf("expected path traversal error on read for %q, got nil", p)
			}
		})
	}
}

func TestVFS_WriteAndRead(t *testing.T) {
	fs := New("/sandbox", DefaultConfig())

	payload := []byte("hello virtual world")
	err := fs.WriteFile("data/file.txt", payload)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	data, err := fs.ReadFile("data/file.txt")
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("data mismatch: expected %q, got %q", string(payload), string(data))
	}

	// Stat check
	info, err := fs.Stat("data/file.txt")
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.SizeBytes != int64(len(payload)) {
		t.Fatalf("expected size %d, got %d", len(payload), info.SizeBytes)
	}
}

func TestVFS_Quotas(t *testing.T) {
	cfg := Config{
		MaxTotalBytes: 50,
		MaxFileCount:  2,
	}
	fs := New("/tenant-quota", cfg)

	// File 1 (30 bytes)
	err := fs.WriteFile("f1.txt", bytes.Repeat([]byte("A"), 30))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File 2 (30 bytes -> total 60 > 50) -> quota exceeded
	err = fs.WriteFile("f2.txt", bytes.Repeat([]byte("B"), 30))
	if !errors.Is(err, ErrStorageQuotaExceeded) {
		t.Fatalf("expected ErrStorageQuotaExceeded, got: %v", err)
	}

	// File 2 (10 bytes -> total 40 <= 50) -> succeeds
	err = fs.WriteFile("f2.txt", bytes.Repeat([]byte("B"), 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File 3 (1 byte) -> file count exceeded (2 files already exist)
	err = fs.WriteFile("f3.txt", []byte("C"))
	if !errors.Is(err, ErrFileCountExceeded) {
		t.Fatalf("expected ErrFileCountExceeded, got: %v", err)
	}

	// Overwriting f2.txt with 15 bytes (diff +5 -> total 45 <= 50) succeeds
	err = fs.WriteFile("f2.txt", bytes.Repeat([]byte("B"), 15))
	if err != nil {
		t.Fatalf("failed to overwrite f2: %v", err)
	}

	// Overwriting f2.txt with 30 bytes (diff +15 -> total 60 > 50) fails
	err = fs.WriteFile("f2.txt", bytes.Repeat([]byte("B"), 30))
	if !errors.Is(err, ErrStorageQuotaExceeded) {
		t.Fatalf("expected ErrStorageQuotaExceeded on overwrite, got %v", err)
	}

	// Remove f1.txt (reclaims 30 bytes, 1 file freed)
	err = fs.RemoveFile("f1.txt")
	if err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	totalBytes, count := fs.TotalUsage()
	if totalBytes != 15 || count != 1 {
		t.Fatalf("expected 15 bytes and 1 file, got %d bytes and %d files", totalBytes, count)
	}

	// Now f3.txt can be created
	err = fs.WriteFile("f3.txt", []byte("C"))
	if err != nil {
		t.Fatalf("unexpected error after freeing quota: %v", err)
	}
}

func TestVFS_ListFiles(t *testing.T) {
	fs := New("/root", DefaultConfig())

	_ = fs.WriteFile("docs/readme.md", []byte("# Readme"))
	_ = fs.WriteFile("docs/guide.md", []byte("# Guide"))
	_ = fs.WriteFile("src/main.go", []byte("package main"))

	docs := fs.ListFiles("docs")
	if len(docs) != 2 {
		t.Fatalf("expected 2 files under docs, got %d", len(docs))
	}

	all := fs.ListFiles("")
	if len(all) != 3 {
		t.Fatalf("expected 3 files under root, got %d", len(all))
	}
}

func TestVFS_ConcurrentAccess(t *testing.T) {
	fs := New("/concurrent", DefaultConfig())
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := fmt.Sprintf("tenant/file_%d.txt", idx)
			data := []byte(fmt.Sprintf("content_%d", idx))

			if err := fs.WriteFile(p, data); err != nil {
				t.Errorf("write error: %v", err)
				return
			}
			read, err := fs.ReadFile(p)
			if err != nil || !bytes.Equal(read, data) {
				t.Errorf("read mismatch: %v", err)
				return
			}
			if _, err := fs.Stat(p); err != nil {
				t.Errorf("stat error: %v", err)
				return
			}
		}(i)
	}

	wg.Wait()
	_, count := fs.TotalUsage()
	if count != 20 {
		t.Fatalf("expected 20 files, got %d", count)
	}
}

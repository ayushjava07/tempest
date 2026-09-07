package blobstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func testBackendContract(t *testing.T, b Backend) {
	ctx := context.Background()

	// Put
	data := []byte("Hello, Tempest Blob Storage Engine!")
	meta, err := b.Put(ctx, "artifacts/build-1/output.log", bytes.NewReader(data), int64(len(data)), map[string]string{"env": "ci"})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if meta.Size != int64(len(data)) {
		t.Fatalf("size mismatch: expected %d, got %d", len(data), meta.Size)
	}

	// Exists
	exists, err := b.Exists(ctx, "artifacts/build-1/output.log")
	if err != nil || !exists {
		t.Fatalf("expected blob to exist")
	}

	// Get
	rc, readMeta, err := b.Get(ctx, "artifacts/build-1/output.log")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer rc.Close()

	readData, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatalf("data mismatch: expected %q, got %q", string(data), string(readData))
	}
	if readMeta.Size != int64(len(data)) {
		t.Fatalf("metadata size mismatch: %d vs %d", readMeta.Size, len(data))
	}

	// GetRange (bytes 7-14 -> "Tempest")
	rangeRC, err := b.GetRange(ctx, "artifacts/build-1/output.log", 7, 7)
	if err != nil {
		t.Fatalf("GetRange failed: %v", err)
	}
	defer rangeRC.Close()
	rangeData, err := io.ReadAll(rangeRC)
	if err != nil {
		t.Fatalf("range read failed: %v", err)
	}
	if string(rangeData) != "Tempest" {
		t.Fatalf("expected 'Tempest', got %q", string(rangeData))
	}

	// List
	list, err := b.List(ctx, "artifacts/build-1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 item in list, got %d", len(list))
	}

	// Delete
	err = b.Delete(ctx, "artifacts/build-1/output.log")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	exists, err = b.Exists(ctx, "artifacts/build-1/output.log")
	if err != nil || exists {
		t.Fatalf("expected blob not to exist after deletion")
	}
}

func TestMemoryBackend(t *testing.T) {
	mem := NewMemoryBackend()
	testBackendContract(t, mem)
}

func TestFSBackend(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tempest-blobstore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fs, err := NewFSBackend(tmpDir)
	if err != nil {
		t.Fatalf("NewFSBackend failed: %v", err)
	}

	testBackendContract(t, fs)
}

func TestMultipartCoordinator(t *testing.T) {
	mem := NewMemoryBackend()
	coord := NewMultipartCoordinator(mem)
	ctx := context.Background()

	key := "large-objects/bundle.tar.gz"
	uploadID, err := coord.CreateMultipart(key)
	if err != nil {
		t.Fatalf("CreateMultipart failed: %v", err)
	}

	part1Data := []byte("chunk-one-")
	part2Data := []byte("chunk-two-")
	part3Data := []byte("chunk-three")

	p1, err := coord.UploadPart(uploadID, 1, bytes.NewReader(part1Data))
	if err != nil {
		t.Fatalf("UploadPart 1 failed: %v", err)
	}

	p2, err := coord.UploadPart(uploadID, 2, bytes.NewReader(part2Data))
	if err != nil {
		t.Fatalf("UploadPart 2 failed: %v", err)
	}

	p3, err := coord.UploadPart(uploadID, 3, bytes.NewReader(part3Data))
	if err != nil {
		t.Fatalf("UploadPart 3 failed: %v", err)
	}

	// Complete multipart with parts out of order to verify re-sorting
	meta, err := coord.CompleteMultipart(ctx, uploadID, []PartInfo{p2, p3, p1})
	if err != nil {
		t.Fatalf("CompleteMultipart failed: %v", err)
	}

	expectedTotal := append(append(part1Data, part2Data...), part3Data...)
	if meta.Size != int64(len(expectedTotal)) {
		t.Fatalf("expected final size %d, got %d", len(expectedTotal), meta.Size)
	}

	rc, _, err := mem.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get final object failed: %v", err)
	}
	defer rc.Close()

	finalData, _ := io.ReadAll(rc)
	if !bytes.Equal(finalData, expectedTotal) {
		t.Fatalf("stitched content mismatch: expected %q, got %q", string(expectedTotal), string(finalData))
	}
}

func TestMultipartCoordinator_Abort(t *testing.T) {
	mem := NewMemoryBackend()
	coord := NewMultipartCoordinator(mem)

	uploadID, _ := coord.CreateMultipart("aborted.bin")
	_, _ = coord.UploadPart(uploadID, 1, bytes.NewReader([]byte("temp")))

	err := coord.AbortMultipart(uploadID)
	if err != nil {
		t.Fatalf("AbortMultipart failed: %v", err)
	}

	// Aborting again should return ErrUploadNotFound
	err = coord.AbortMultipart(uploadID)
	if err != ErrUploadNotFound {
		t.Fatalf("expected ErrUploadNotFound on aborted session, got: %v", err)
	}
}

func TestBlobStore_ConcurrentAccess(t *testing.T) {
	mem := NewMemoryBackend()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("tenant/obj_%d.dat", idx)
			data := []byte(fmt.Sprintf("payload-%d", idx))

			_, err := mem.Put(ctx, key, bytes.NewReader(data), int64(len(data)), nil)
			if err != nil {
				t.Errorf("Put failed: %v", err)
				return
			}

			rc, _, err := mem.Get(ctx, key)
			if err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}
			defer rc.Close()

			got, err := io.ReadAll(rc)
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("read mismatch: %v", err)
				return
			}
		}(i)
	}

	wg.Wait()
}

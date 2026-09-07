package archiver

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func sampleRecords(n int) []ArchiveRecord {
	records := make([]ArchiveRecord, n)
	for i := 0; i < n; i++ {
		records[i] = ArchiveRecord{
			RunID:      fmt.Sprintf("run-%04d", i),
			WorkflowID: "order-processing",
			Namespace:  "production",
			FinalState: "SUCCEEDED",
			StartedAt:  time.Now().Add(-time.Hour),
			FinishedAt: time.Now(),
			Variables:  map[string]string{"customer_id": fmt.Sprintf("cust-%d", i), "region": "us-east-1"},
			Steps: []StepArchive{
				{StepID: "validate", State: "SUCCEEDED", ExitCode: 0},
				{StepID: "charge", State: "SUCCEEDED", ExitCode: 0},
				{StepID: "fulfill", State: "SUCCEEDED", ExitCode: 0},
			},
			PayloadJSON: []byte(`{"items":[{"sku":"item-1","qty":2},{"sku":"item-2","qty":1}]}`),
		}
	}
	return records
}

func TestArchiver_RoundTripNone(t *testing.T) {
	records := sampleRecords(10)

	exporter := NewExporter()
	var buf bytes.Buffer
	manifest, err := exporter.Export(records, CodecNone, &buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if manifest.TotalRecords != 10 {
		t.Fatalf("expected 10 records, got %d", manifest.TotalRecords)
	}

	unpacker := NewUnpacker()
	unpacked, err := unpacker.Unpack(&buf, manifest)
	if err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}

	if len(unpacked) != len(records) {
		t.Fatalf("record count mismatch: %d vs %d", len(unpacked), len(records))
	}
	if unpacked[0].RunID != records[0].RunID {
		t.Fatalf("first record mismatch: %s vs %s", unpacked[0].RunID, records[0].RunID)
	}
}

func TestArchiver_RoundTripGzipAndCompressionRatio(t *testing.T) {
	records := sampleRecords(50)

	exporter := NewExporter()
	var buf bytes.Buffer
	manifest, err := exporter.Export(records, CodecGzip, &buf)
	if err != nil {
		t.Fatalf("Export gzip failed: %v", err)
	}

	// Gzip must achieve positive compression ratio
	if manifest.CompressedBytes >= manifest.UncompressedBytes {
		t.Fatalf("expected compression reduction, got compressed=%d uncompressed=%d",
			manifest.CompressedBytes, manifest.UncompressedBytes)
	}

	unpacker := NewUnpacker()
	unpacked, err := unpacker.Unpack(&buf, manifest)
	if err != nil {
		t.Fatalf("Unpack gzip failed: %v", err)
	}

	if len(unpacked) != 50 {
		t.Fatalf("expected 50 unpacked records, got %d", len(unpacked))
	}
}

func TestArchiver_Index(t *testing.T) {
	records := sampleRecords(20)
	idx := BuildIndex(records)

	entry, ok := idx.Entries["run-0005"]
	if !ok {
		t.Fatalf("expected to find run-0005 in index")
	}
	if entry.Index != 5 || entry.WorkflowID != "order-processing" {
		t.Fatalf("unexpected index entry: %+v", entry)
	}

	var buf bytes.Buffer
	if err := idx.Write(&buf); err != nil {
		t.Fatalf("Write index failed: %v", err)
	}

	readIdx, err := ReadIndex(&buf)
	if err != nil {
		t.Fatalf("ReadIndex failed: %v", err)
	}
	if len(readIdx.Entries) != len(records) {
		t.Fatalf("deserialized index length mismatch")
	}
}

func TestArchiver_TamperedManifestRejection(t *testing.T) {
	records := sampleRecords(5)
	exporter := NewExporter()
	var buf bytes.Buffer
	manifest, _ := exporter.Export(records, CodecNone, &buf)

	// Tamper payload byte
	corrupted := buf.Bytes()
	corrupted[len(corrupted)-1] ^= 0xFF

	unpacker := NewUnpacker()
	_, err := unpacker.Unpack(bytes.NewReader(corrupted), manifest)
	if err != ErrChecksumMismatch {
		t.Fatalf("expected ErrChecksumMismatch, got: %v", err)
	}
}

func TestArchiver_TruncatedStreamRejection(t *testing.T) {
	records := sampleRecords(10)
	exporter := NewExporter()
	var buf bytes.Buffer
	manifest, _ := exporter.Export(records, CodecGzip, &buf)

	// Truncate to first 30 bytes
	truncated := buf.Bytes()[:30]

	unpacker := NewUnpacker()
	_, err := unpacker.Unpack(bytes.NewReader(truncated), manifest)
	if err == nil {
		t.Fatalf("expected error on truncated archive stream, got nil")
	}
}

func TestArchiver_RetentionPolicyEvaluation(t *testing.T) {
	now := time.Now()
	records := []ArchiveRecord{
		{RunID: "r1-new", FinishedAt: now.Add(-1 * time.Hour), FinalState: "SUCCEEDED"},
		{RunID: "r2-old", FinishedAt: now.Add(-48 * time.Hour), FinalState: "SUCCEEDED"},
		{RunID: "r3-failed-old", FinishedAt: now.Add(-72 * time.Hour), FinalState: "FAILED"},
		{RunID: "r4-ancient", FinishedAt: now.Add(-100 * time.Hour), FinalState: "SUCCEEDED"},
	}

	policy := RetentionPolicy{
		MaxAge:         24 * time.Hour,
		KeepFailed:     true,
		MinRetainCount: 1, // Keep r1 regardless
	}

	retain, purge := FilterForRetention(records, now, policy)

	// Retain: r1 (new), r3 (failed exception)
	// Purge: r2, r4
	if len(retain) != 2 {
		t.Fatalf("expected 2 retained runs, got %d", len(retain))
	}
	if len(purge) != 2 {
		t.Fatalf("expected 2 purged runs, got %d", len(purge))
	}

	for _, r := range retain {
		if r.RunID == "r2-old" || r.RunID == "r4-ancient" {
			t.Errorf("unexpected run retained: %s", r.RunID)
		}
	}
}

func TestArchiver_ConcurrentExports(t *testing.T) {
	exporter := NewExporter()
	unpacker := NewUnpacker()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			records := sampleRecords(5)
			var buf bytes.Buffer
			manifest, err := exporter.Export(records, CodecGzip, &buf)
			if err != nil {
				t.Errorf("concurrent export error: %v", err)
				return
			}
			unpacked, err := unpacker.Unpack(&buf, manifest)
			if err != nil || len(unpacked) != 5 {
				t.Errorf("concurrent unpack error: %v", err)
				return
			}
		}(i)
	}

	wg.Wait()
}

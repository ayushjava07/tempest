package ledger

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestLedger_ValidChainVerification(t *testing.T) {
	l := NewLedger()

	r1 := l.Append("default", "alice", "workflow:create", "wf-1", []byte(`{"definition":1}`))
	r2 := l.Append("default", "bob", "run:trigger", "run-10", []byte(`{"input":"test"}`))
	r3 := l.Append("prod", "charlie", "run:cancel", "run-10", nil)

	if l.Length() != 4 { // genesis + 3
		t.Fatalf("expected 4 records, got %d", l.Length())
	}

	if r2.PrevHash != r1.BlockHash {
		t.Errorf("r2 PrevHash does not point to r1 BlockHash")
	}
	if r3.PrevHash != r2.BlockHash {
		t.Errorf("r3 PrevHash does not point to r2 BlockHash")
	}

	if err := l.Verify(); err != nil {
		t.Fatalf("valid chain failed verification: %v", err)
	}
}

func TestLedger_TamperedPayloadDetected(t *testing.T) {
	l := NewLedger()

	_ = l.Append("default", "alice", "create", "wf", []byte("secret"))
	_ = l.Append("default", "bob", "update", "wf", []byte("update"))

	// Deliberately tamper with record 1 payload hash
	l.mu.Lock()
	l.records[1].PayloadHash = "0000000000000000000000000000000000000000000000000000000000000000"
	l.mu.Unlock()

	err := l.Verify()
	if !errors.Is(err, ErrTamperedBlock) {
		t.Fatalf("expected ErrTamperedBlock on altered payload hash, got %v", err)
	}
}

func TestLedger_BrokenChainPointerDetected(t *testing.T) {
	l := NewLedger()

	_ = l.Append("default", "alice", "create", "wf", nil)
	_ = l.Append("default", "bob", "update", "wf", nil)

	// Tamper with record 2 previous hash pointer
	l.mu.Lock()
	l.records[2].PrevHash = "fake-prev-hash"
	l.mu.Unlock()

	err := l.Verify()
	if !errors.Is(err, ErrBrokenChain) {
		t.Fatalf("expected ErrBrokenChain on corrupted prevHash pointer, got %v", err)
	}
}

func TestLedger_QueryFiltering(t *testing.T) {
	l := NewLedger()

	_ = l.Append("ns-a", "alice", "create", "wf-1", nil)
	_ = l.Append("ns-a", "bob", "run", "wf-1", nil)
	_ = l.Append("ns-b", "alice", "create", "wf-2", nil)

	res := l.Query("ns-a", "", "", 10)
	if len(res) != 2 {
		t.Errorf("expected 2 records in ns-a, got %d", len(res))
	}

	aliceRes := l.Query("", "alice", "", 10)
	if len(aliceRes) != 2 {
		t.Errorf("expected 2 records by alice, got %d", len(aliceRes))
	}

	createRes := l.Query("", "", "create", 10)
	if len(createRes) != 2 {
		t.Errorf("expected 2 create actions, got %d", len(createRes))
	}
}

func TestLedger_Concurrency(t *testing.T) {
	l := NewLedger()
	concurrency := 10
	recsPerWorker := 20
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recsPerWorker; j++ {
				l.Append("concurrent-ns", fmt.Sprintf("worker-%d", id), "exec", "task", []byte("data"))
			}
		}(i)
	}
	wg.Wait()

	if l.Length() != 1+concurrency*recsPerWorker {
		t.Fatalf("expected %d total records, got %d", 1+concurrency*recsPerWorker, l.Length())
	}

	if err := l.Verify(); err != nil {
		t.Fatalf("concurrently appended chain failed verification: %v", err)
	}
}

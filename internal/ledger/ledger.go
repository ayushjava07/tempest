package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrTamperedBlock = errors.New("ledger: block hash does not match computed hash (tampering detected)")
	ErrBrokenChain   = errors.New("ledger: previous hash pointer broken (chain interrupted)")
	ErrEmptyChain    = errors.New("ledger: cannot verify empty ledger")
)

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Record represents an immutable entry in the audit ledger.
type Record struct {
	Index       uint64
	Timestamp   time.Time
	Namespace   string
	Actor       string
	Action      string
	Resource    string
	PayloadHash string
	PrevHash    string
	BlockHash   string
}

// ComputeHash computes the cryptographic SHA-256 hash of a record given its fields and predecessor.
func ComputeHash(index uint64, ts time.Time, ns, actor, action, resource, payloadHash, prevHash string) string {
	raw := fmt.Sprintf("%d:%d:%s:%s:%s:%s:%s:%s",
		index, ts.UnixNano(), ns, actor, action, resource, payloadHash, prevHash)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// HashPayload generates the SHA-256 digest of arbitrary payload bytes.
func HashPayload(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// Ledger manages an append-only cryptographic hash chain.
type Ledger struct {
	mu      sync.RWMutex
	records []Record
}

func NewLedger() *Ledger {
	l := &Ledger{
		records: make([]Record, 0, 128),
	}
	// Append genesis record
	now := time.Unix(0, 0).UTC()
	genesisHash := ComputeHash(0, now, "system", "system", "GENESIS", "root", GenesisHash, GenesisHash)
	l.records = append(l.records, Record{
		Index:       0,
		Timestamp:   now,
		Namespace:   "system",
		Actor:       "system",
		Action:      "GENESIS",
		Resource:    "root",
		PayloadHash: GenesisHash,
		PrevHash:    GenesisHash,
		BlockHash:   genesisHash,
	})
	return l
}

// Append records a new audit event and locks it into the cryptographic chain.
func (l *Ledger) Append(ns, actor, action, resource string, payload []byte) Record {
	l.mu.Lock()
	defer l.mu.Unlock()

	last := l.records[len(l.records)-1]
	newIdx := last.Index + 1
	now := time.Now().UTC()
	pHash := HashPayload(payload)
	prevHash := last.BlockHash

	blockHash := ComputeHash(newIdx, now, ns, actor, action, resource, pHash, prevHash)

	rec := Record{
		Index:       newIdx,
		Timestamp:   now,
		Namespace:   ns,
		Actor:       actor,
		Action:      action,
		Resource:    resource,
		PayloadHash: pHash,
		PrevHash:    prevHash,
		BlockHash:   blockHash,
	}

	l.records = append(l.records, rec)
	return rec
}

// Verify walks the entire chain from genesis to head and verifies all cryptographic hashes.
func (l *Ledger) Verify() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.records) == 0 {
		return ErrEmptyChain
	}

	for i := 1; i < len(l.records); i++ {
		prev := l.records[i-1]
		curr := l.records[i]

		if curr.PrevHash != prev.BlockHash {
			return fmt.Errorf("%w: record %d PrevHash %s != record %d BlockHash %s",
				ErrBrokenChain, curr.Index, curr.PrevHash, prev.Index, prev.BlockHash)
		}

		expectedHash := ComputeHash(curr.Index, curr.Timestamp, curr.Namespace,
			curr.Actor, curr.Action, curr.Resource, curr.PayloadHash, curr.PrevHash)

		if curr.BlockHash != expectedHash {
			return fmt.Errorf("%w: record %d BlockHash %s != computed %s",
				ErrTamperedBlock, curr.Index, curr.BlockHash, expectedHash)
		}
	}

	return nil
}

// Length returns the current number of records in the ledger.
func (l *Ledger) Length() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.records)
}

// Query retrieves records matching the filter criteria.
func (l *Ledger) Query(ns, actor, action string, limit int) []Record {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var out []Record
	for i := len(l.records) - 1; i >= 0; i-- {
		r := l.records[i]
		if ns != "" && r.Namespace != ns {
			continue
		}
		if actor != "" && r.Actor != actor {
			continue
		}
		if action != "" && r.Action != action {
			continue
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

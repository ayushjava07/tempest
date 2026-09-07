package fastjson

import (
	"encoding/json"
	"fmt"
	"sync"
)

// StringInterner retains unique string instances to avoid heap allocation on repeated JSON dictionary keys.
type StringInterner struct {
	mu    sync.RWMutex
	table map[string]string
}

// NewStringInterner creates a string interner pre-seeded with common workflow event keys.
func NewStringInterner() *StringInterner {
	interner := &StringInterner{
		table: make(map[string]string),
	}

	preseeded := []string{
		"seq", "type", "step_id", "payload", "status", "run_id", "workflow_id",
		"timestamp", "data", "error", "action", "retry", "timeout", "success",
		"failure", "pending", "running", "completed", "cancelled",
	}

	for _, s := range preseeded {
		interner.table[s] = s
	}

	return interner
}

// Intern returns a canonical interned string for the provided byte slice.
func (in *StringInterner) Intern(b []byte) string {
	in.mu.RLock()
	s, exists := in.table[string(b)]
	in.mu.RUnlock()

	if exists {
		return s
	}

	in.mu.Lock()
	defer in.mu.Unlock()

	// Double check
	str := string(b)
	if existing, ok := in.table[str]; ok {
		return existing
	}
	in.table[str] = str
	return str
}

// Global default interner
var defaultInterner = NewStringInterner()

// Intern returns an interned string from the default interner.
func Intern(b []byte) string {
	return defaultInterner.Intern(b)
}

// DecodedEvent represents an unmarshaled event with interned keys.
type DecodedEvent struct {
	Seq       uint64
	EventType string
	StepID    string
	Payload   map[string]string
}

// DecodeWorkflowEvent parses JSON bytes into a DecodedEvent with interned string keys.
func DecodeWorkflowEvent(data []byte) (*DecodedEvent, error) {
	// Use standard json raw message map to populate interned keys
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse event json: %w", err)
	}

	evt := &DecodedEvent{
		Payload: make(map[string]string),
	}

	if rawSeq, ok := raw[defaultInterner.Intern([]byte("seq"))]; ok {
		_ = json.Unmarshal(rawSeq, &evt.Seq)
	}

	if rawType, ok := raw[defaultInterner.Intern([]byte("type"))]; ok {
		var s string
		if err := json.Unmarshal(rawType, &s); err == nil {
			evt.EventType = defaultInterner.Intern([]byte(s))
		}
	}

	if rawStep, ok := raw[defaultInterner.Intern([]byte("step_id"))]; ok {
		var s string
		if err := json.Unmarshal(rawStep, &s); err == nil {
			evt.StepID = defaultInterner.Intern([]byte(s))
		}
	}

	if rawPayload, ok := raw[defaultInterner.Intern([]byte("payload"))]; ok {
		var m map[string]string
		if err := json.Unmarshal(rawPayload, &m); err == nil {
			for k, v := range m {
				internedKey := defaultInterner.Intern([]byte(k))
				evt.Payload[internedKey] = v
			}
		}
	}

	return evt, nil
}

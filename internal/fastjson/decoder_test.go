package fastjson

import (
	"fmt"
	"sync"
	"testing"
	"unsafe"
)

func TestStringInterningPointers(t *testing.T) {
	in := NewStringInterner()

	b1 := []byte("status")
	b2 := []byte("status")

	s1 := in.Intern(b1)
	s2 := in.Intern(b2)

	if s1 != s2 {
		t.Fatalf("expected string values to match: %s != %s", s1, s2)
	}

	// Verify pointer identity in memory
	ptr1 := unsafe.StringData(s1)
	ptr2 := unsafe.StringData(s2)
	if ptr1 != ptr2 {
		t.Fatalf("expected interned strings to share identical underlying byte buffer pointer")
	}

	// Dynamically added key
	dyn1 := in.Intern([]byte("custom_dynamic_key_999"))
	dyn2 := in.Intern([]byte("custom_dynamic_key_999"))
	if unsafe.StringData(dyn1) != unsafe.StringData(dyn2) {
		t.Fatalf("expected dynamically interned string to share pointer")
	}
}

func TestDecodeWorkflowEvent(t *testing.T) {
	raw := []byte(`{
		"seq": 500,
		"type": "StepStarted",
		"step_id": "step-process",
		"payload": {
			"status": "running",
			"action": "execute"
		}
	}`)

	evt, err := DecodeWorkflowEvent(raw)
	if err != nil {
		t.Fatalf("DecodeWorkflowEvent failed: %v", err)
	}

	if evt.Seq != 500 {
		t.Fatalf("expected seq 500, got %d", evt.Seq)
	}
	if evt.EventType != "StepStarted" {
		t.Fatalf("expected StepStarted, got %s", evt.EventType)
	}
	if evt.StepID != "step-process" {
		t.Fatalf("expected step-process, got %s", evt.StepID)
	}
	if evt.Payload["status"] != "running" || evt.Payload["action"] != "execute" {
		t.Fatalf("unexpected payload: %v", evt.Payload)
	}
}

func TestConcurrentInterning(t *testing.T) {
	in := NewStringInterner()
	var wg sync.WaitGroup

	const goroutines = 20
	const perRoutine = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for j := 0; j < perRoutine; j++ {
				key := fmt.Sprintf("shared_key_%d", j%10)
				_ = in.Intern([]byte(key))
			}
		}(i)
	}

	wg.Wait()
}

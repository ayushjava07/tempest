package audit

import (
	"context"
	"testing"
)

func TestLog_Record(t *testing.T) {
	log := NewLog(100)
	log.Record(Entry{
		Principal: "user1",
		Action:    "submit",
		Resource:  "run",
		Outcome:   "success",
	})
	if log.Size() != 1 {
		t.Errorf("expected 1 entry, got %d", log.Size())
	}
}

func TestLog_MaxSize(t *testing.T) {
	log := NewLog(3)
	for i := 0; i < 10; i++ {
		log.Record(Entry{Action: "test"})
	}
	if log.Size() != 3 {
		t.Errorf("expected 3 entries, got %d", log.Size())
	}
}

func TestLog_List(t *testing.T) {
	log := NewLog(100)
	for i := 0; i < 5; i++ {
		log.Record(Entry{Action: "test"})
	}
	entries := log.List(3)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestLog_List_All(t *testing.T) {
	log := NewLog(100)
	log.Record(Entry{Action: "a"})
	log.Record(Entry{Action: "b"})
	entries := log.List(0)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestLog_Clear(t *testing.T) {
	log := NewLog(100)
	log.Record(Entry{Action: "test"})
	log.Clear()
	if log.Size() != 0 {
		t.Error("expected cleared log")
	}
}

func TestLog_Timestamp(t *testing.T) {
	log := NewLog(100)
	log.Record(Entry{Action: "test"})
	entries := log.List(1)
	if entries[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestLog_ThreadSafety(t *testing.T) {
	log := NewLog(100)
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			log.Record(Entry{Action: "test"})
			log.List(10)
			done <- true
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	if log.Size() != 100 {
		t.Errorf("expected 100 entries, got %d", log.Size())
	}
}

func TestContextLog(t *testing.T) {
	log := NewLog(100)
	ctx := WithLog(context.Background(), log)
	got, ok := LogFrom(ctx)
	if !ok || got != log {
		t.Error("expected to retrieve log from context")
	}
}

func TestRecordFrom_Context(t *testing.T) {
	log := NewLog(100)
	ctx := WithLog(context.Background(), log)
	RecordFrom(ctx, Entry{Action: "test"})
	if log.Size() != 1 {
		t.Errorf("expected 1 entry, got %d", log.Size())
	}
}

func TestRecordFrom_NoContext(t *testing.T) {
	RecordFrom(nil, Entry{Action: "test"})
}

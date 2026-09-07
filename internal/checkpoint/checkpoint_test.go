package checkpoint

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_SaveLoad(t *testing.T) {
	ms := NewMemoryStore()
	cp := &Checkpoint{ID: "1", Name: "test", State: State{"x": 1}}
	if err := ms.Save(context.Background(), cp); err != nil {
		t.Fatal(err)
	}
	loaded, err := ms.Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "test" {
		t.Error("name mismatch")
	}
}

func TestMemoryStore_List(t *testing.T) {
	ms := NewMemoryStore()
	ms.Save(context.Background(), &Checkpoint{ID: "1", Name: "a", State: State{}})
	ms.Save(context.Background(), &Checkpoint{ID: "2", Name: "b", State: State{}})
	list, err := ms.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	ms := NewMemoryStore()
	ms.Save(context.Background(), &Checkpoint{ID: "1", Name: "test", State: State{}})
	if err := ms.Delete(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Load(context.Background(), "1"); err == nil {
		t.Error("expected not found")
	}
}

func TestManager_CreateRestore(t *testing.T) {
	m := NewManager(NewMemoryStore())
	state := State{"counter": 42, "status": "running"}
	cp, err := m.Create(context.Background(), "my-checkpoint", state)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := m.Restore(context.Background(), cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	v := restored["counter"]
	switch v := v.(type) {
	case int:
		if v != 42 {
			t.Errorf("state mismatch: got %d", v)
		}
	case float64:
		if v != 42 {
			t.Errorf("state mismatch: got %v", v)
		}
	default:
		t.Errorf("state mismatch: unexpected type %T", v)
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager(NewMemoryStore())
	m.Create(context.Background(), "a", State{})
	m.Create(context.Background(), "b", State{})
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestCheckpoint_SerializeDeserialize(t *testing.T) {
	cp := &Checkpoint{
		ID:        "test-1",
		Name:      "checkpoint",
		State:     State{"key": "value"},
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := cp.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Deserialize(data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != "test-1" || restored.Name != "checkpoint" {
		t.Error("deserialize mismatch")
	}
}

func TestCheckpoint_NotFound(t *testing.T) {
	m := NewManager(NewMemoryStore())
	_, err := m.Restore(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error")
	}
}
package artifact

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryStore_SaveLoad(t *testing.T) {
	ms := NewMemoryStore()
	art := &Artifact{ID: "test-1", Name: "test.txt", ContentType: "text/plain"}
	data := strings.NewReader("hello world")
	if err := ms.Save(context.Background(), art, data); err != nil {
		t.Fatal(err)
	}
	loaded, reader, err := ms.Load(context.Background(), "test-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "test.txt" {
		t.Error("name mismatch")
	}
	buf := make([]byte, loaded.Size)
	reader.Read(buf)
	if string(buf) != "hello world" {
		t.Error("data mismatch")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	ms := NewMemoryStore()
	art := &Artifact{ID: "del-1", Name: "del.txt"}
	ms.Save(context.Background(), art, strings.NewReader("data"))
	if err := ms.Delete(context.Background(), "del-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ms.Load(context.Background(), "del-1"); err == nil {
		t.Error("expected not found")
	}
}

func TestMemoryStore_List(t *testing.T) {
	ms := NewMemoryStore()
	ms.Save(context.Background(), &Artifact{ID: "a", Name: "a.txt"}, strings.NewReader("a"))
	ms.Save(context.Background(), &Artifact{ID: "b", Name: "b.txt"}, strings.NewReader("b"))
	list, err := ms.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestManager_Store(t *testing.T) {
	m := NewManager(NewMemoryStore())
	art := &Artifact{Name: "managed.txt", ContentType: "text/plain"}
	if err := m.Store(context.Background(), art, strings.NewReader("managed")); err != nil {
		t.Fatal(err)
	}
	if art.ID == "" {
		t.Error("expected generated ID")
	}
}

func TestManager_Get(t *testing.T) {
	m := NewManager(NewMemoryStore())
	art := &Artifact{ID: "get-1", Name: "get.txt"}
	m.Store(context.Background(), art, strings.NewReader("data"))
	loaded, reader, err := m.Get(context.Background(), "get-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "get-1" {
		t.Error("ID mismatch")
	}
	buf := make([]byte, loaded.Size)
	reader.Read(buf)
	if string(buf) != "data" {
		t.Error("data mismatch")
	}
}

func TestArtifact_Checksum(t *testing.T) {
	ms := NewMemoryStore()
	art := &Artifact{ID: "check-1", Name: "check.txt"}
	ms.Save(context.Background(), art, strings.NewReader("test data"))
	if art.Checksum == "" {
		t.Error("expected checksum")
	}
	if len(art.Checksum) != 64 {
		t.Errorf("expected 64 char checksum, got %d", len(art.Checksum))
	}
}
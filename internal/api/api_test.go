package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/auth"
	"github.com/tempest-io/tempest/internal/persistence/memstore"
	"github.com/tempest-io/tempest/internal/scheduler"
	v1 "github.com/tempest-io/tempest/internal/api/v1"
)

func setupServer(t *testing.T) (*Server, *memstore.Store) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := memstore.NewWithClock(func() time.Time { return now })
	resolver := auth.NewResolver(store)
	engine := scheduler.NewEngine(store, nil, scheduler.Options{
		Clock: func() time.Time { return now },
	})
	srv := NewServer(ServerOptions{
		Store:    store,
		Resolver: resolver,
		Engine:   engine,
		Addr:     ":0",
	})
	return srv, store
}

func TestReadyz(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest("GET", "/v1/readyz", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListRuns_Empty(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest("GET", "/v1/runs", nil)
	ctx := context.WithValue(req.Context(), namespaceKey{}, "default")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.listRuns(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp v1.ListResponse[v1.Run]
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 items, got %d", resp.Total)
	}
}

func TestListDefinitions_Empty(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest("GET", "/v1/registry/definitions", nil)
	ctx := context.WithValue(req.Context(), namespaceKey{}, "default")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.listDefinitions(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCreateDefinition(t *testing.T) {
	srv, _ := setupServer(t)
	def := v1.WorkflowDefinition{
		Name:      "test-wf",
		Version:   1,
		Namespace: "default",
		Steps:     []v1.Step{{ID: "s1", Handler: "pass"}},
	}
	body, _ := json.Marshal(def)
	req := httptest.NewRequest("POST", "/v1/registry/definitions", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), namespaceKey{}, "default")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.createDefinition(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDefinition_NotFound(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest("GET", "/v1/registry/definitions/nonexistent", nil)
	req.SetPathValue("name", "nonexistent")
	ctx := context.WithValue(req.Context(), namespaceKey{}, "default")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.getDefinition(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest("GET", "/v1/runs/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	ctx := context.WithValue(req.Context(), namespaceKey{}, "default")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.getRun(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMigrate(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest("POST", "/v1/migrate", nil)
	w := httptest.NewRecorder()
	srv.migrate(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tempest-io/tempest/internal/persistence/memstore"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func TestDashboardRenders(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := memstore.NewWithClock(func() time.Time { return now })
	ctx := context.Background()
	_ = store.CreateDefinition(ctx, &ttypes.WorkflowDefinition{
		ID:        ttypes.WorkflowID{Namespace: "default", Name: "test-wf", Version: 1},
		Steps:     []ttypes.StepDefinition{{ID: "s1", Handler: "pass"}},
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = store.CreateRun(ctx, &ttypes.Run{
		ID: "run-1", Namespace: "default",
		Workflow: ttypes.WorkflowID{Name: "test-wf", Version: 1},
		State: ttypes.StateRunning, CreatedAt: now,
	})
	srv := NewServer(store)
	req := httptest.NewRequest("GET", "/?namespace=default", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestDashboard_Empty(t *testing.T) {
	store := memstore.New()
	srv := NewServer(store)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

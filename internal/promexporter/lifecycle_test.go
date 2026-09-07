package promexporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLifecycleServerGracefulStartAndStop(t *testing.T) {
	reg := NewRegistry()
	c := NewCounter("test_counter", "sample counter", nil)
	c.Add(99)
	reg.RegisterCounter(c)

	cfg := ServerConfig{
		Addr:            ":0",
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     5 * time.Second,
		ShutdownTimeout: 2 * time.Second,
	}

	server := NewLifecycleServer(cfg, reg, nil)
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start lifecycle server: %v", err)
	}

	if !server.IsRunning() {
		t.Fatalf("expected server.IsRunning() to be true")
	}

	addr := server.Addr()
	if addr == "" {
		t.Fatalf("expected non-empty addr")
	}

	// Verify server handler serves /metrics directly
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "test_counter 99") {
		t.Fatalf("missing metric in response body: %s", rec.Body.String())
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		t.Fatalf("graceful stop failed: %v", err)
	}

	if server.IsRunning() {
		t.Fatalf("expected server.IsRunning() to be false after stop")
	}
}

package promexporter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzEndpointEvaluation(t *testing.T) {
	health := NewHealthRegistry()
	reg := NewRegistry()

	// 1. All healthy
	health.RegisterCheck("database", true, func(ctx context.Context) (HealthStatus, string) {
		return StatusHealthy, "connected"
	})
	health.RegisterCheck("cache", false, func(ctx context.Context) (HealthStatus, string) {
		return StatusHealthy, "operational"
	})

	handler := Handler(reg, health)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "HEALTHY" {
		t.Fatalf("expected HEALTHY status, got %v", resp["status"])
	}

	// 2. Non-critical dependency fails -> DEGRADED (still 200 OK)
	health.RegisterCheck("cache", false, func(ctx context.Context) (HealthStatus, string) {
		return StatusDegraded, "high latency"
	})

	recDegraded := httptest.NewRecorder()
	handler.ServeHTTP(recDegraded, req)

	if recDegraded.Code != http.StatusOK {
		t.Fatalf("degraded non-critical service should return 200, got %d", recDegraded.Code)
	}
	_ = json.Unmarshal(recDegraded.Body.Bytes(), &resp)
	if resp["status"] != "DEGRADED" {
		t.Fatalf("expected DEGRADED, got %v", resp["status"])
	}

	// 3. Critical dependency fails -> UNHEALTHY (HTTP 503)
	health.RegisterCheck("database", true, func(ctx context.Context) (HealthStatus, string) {
		return StatusUnhealthy, "connection refused"
	})

	recUnhealthy := httptest.NewRecorder()
	handler.ServeHTTP(recUnhealthy, req)

	if recUnhealthy.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", recUnhealthy.Code)
	}
	_ = json.Unmarshal(recUnhealthy.Body.Bytes(), &resp)
	if resp["status"] != "UNHEALTHY" {
		t.Fatalf("expected UNHEALTHY status, got %v", resp["status"])
	}
}

func TestMetricsEndpointScrape(t *testing.T) {
	reg := NewRegistry()
	c := NewCounter("tempest_tasks_total", "Tasks executed", map[string]string{"worker": "w1"})
	c.Inc()
	reg.RegisterCounter(c)

	handler := Handler(reg, nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %s", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `tempest_tasks_total{worker="w1"} 1`) {
		t.Fatalf("expected metric in response, got:\n%s", body)
	}
}

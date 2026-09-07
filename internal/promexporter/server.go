package promexporter

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the overall readiness condition of the service.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "HEALTHY"
	StatusDegraded  HealthStatus = "DEGRADED"
	StatusUnhealthy HealthStatus = "UNHEALTHY"
)

// SubsystemHealth details health state of an individual dependency.
type SubsystemHealth struct {
	Name      string       `json:"name"`
	Status    HealthStatus `json:"status"`
	Critical  bool         `json:"critical"`
	Message   string       `json:"message,omitempty"`
	CheckedAt time.Time    `json:"checked_at"`
}

// HealthCheckFunc is invoked by the readiness handler to determine subsystem state.
type HealthCheckFunc func(ctx context.Context) (HealthStatus, string)

// HealthRegistry collects subsystem health checks for /healthz.
type HealthRegistry struct {
	mu     sync.RWMutex
	checks map[string]struct {
		critical bool
		fn       HealthCheckFunc
	}
}

// NewHealthRegistry creates a health check registry.
func NewHealthRegistry() *HealthRegistry {
	return &HealthRegistry{
		checks: make(map[string]struct {
			critical bool
			fn       HealthCheckFunc
		}),
	}
}

// RegisterCheck adds a subsystem check to the registry.
func (h *HealthRegistry) RegisterCheck(name string, critical bool, fn HealthCheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = struct {
		critical bool
		fn       HealthCheckFunc
	}{critical: critical, fn: fn}
}

// Evaluate evaluates all subsystem checks.
func (h *HealthRegistry) Evaluate(ctx context.Context) (HealthStatus, []SubsystemHealth) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	overall := StatusHealthy
	var reports []SubsystemHealth

	for name, item := range h.checks {
		st, msg := item.fn(ctx)
		report := SubsystemHealth{
			Name:      name,
			Status:    st,
			Critical:  item.critical,
			Message:   msg,
			CheckedAt: time.Now().UTC(),
		}
		reports = append(reports, report)

		if st == StatusUnhealthy && item.critical {
			overall = StatusUnhealthy
		} else if st != StatusHealthy && overall == StatusHealthy {
			overall = StatusDegraded
		}
	}

	return overall, reports
}

// Handler returns an http.Handler exposing /metrics and /healthz.
func Handler(registry *CollectorRegistry, health *HealthRegistry) http.Handler {
	mux := http.NewServeMux()

	// /metrics: Prometheus text exposition
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(registry.Gather())
	})

	// /healthz: Readiness / Liveness probe
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var overall HealthStatus = StatusHealthy
		var reports []SubsystemHealth

		if health != nil {
			overall, reports = health.Evaluate(ctx)
		}

		resp := map[string]any{
			"status":     overall,
			"subsystems": reports,
			"timestamp":  time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		if overall == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		_ = json.NewEncoder(w).Encode(resp)
	})

	return mux
}

// NewServer configures an HTTP server exposing /metrics and /healthz.
func NewServer(addr string, reg *CollectorRegistry, health *HealthRegistry) *http.Server {
	if addr == "" {
		addr = ":9090"
	}
	return &http.Server{
		Addr:         addr,
		Handler:      Handler(reg, health),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

package promexporter

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServerConfig configures telemetry HTTP server listening and timeouts.
type ServerConfig struct {
	Addr            string        `json:"addr"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

// DefaultServerConfig provides robust production defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Addr:            ":9090",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}

// LifecycleServer manages graceful startup and shutdown of telemetry endpoints.
type LifecycleServer struct {
	cfg      ServerConfig
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewLifecycleServer creates a managed HTTP telemetry server.
func NewLifecycleServer(cfg ServerConfig, reg *CollectorRegistry, health *HealthRegistry) *LifecycleServer {
	if cfg.ReadTimeout <= 0 {
		cfg = DefaultServerConfig()
	}

	handler := Handler(reg, health)
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &LifecycleServer{
		cfg:    cfg,
		server: srv,
	}
}

// Start binds to the configured network address and serves requests.
func (s *LifecycleServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.listener = ln
	s.running = true
	s.mu.Unlock()

	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}
	}()

	return nil
}

// Addr returns the bound network address (useful when using dynamic port :0).
func (s *LifecycleServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Addr
}

// IsRunning returns true if the server is actively serving.
func (s *LifecycleServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Stop gracefully shuts down the server within the shutdown timeout.
func (s *LifecycleServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
	}

	return s.server.Shutdown(ctx)
}

package api

import (
	"net/http"

	"github.com/tempest-io/tempest/internal/auth"
)

func NewMux(server *Server) http.Handler {
	mux := http.NewServeMux()
	s := &sub{server: server}
	authRead := RequireAuth(server.resolver, auth.ActionRead)
	authSubmit := RequireAuth(server.resolver, auth.ActionSubmit)
	authCancel := RequireAuth(server.resolver, auth.ActionCancel)
	authPublish := RequireAuth(server.resolver, auth.ActionPublish)
	authWebhooks := RequireAuth(server.resolver, auth.ActionManageWebhooks)

	mux.Handle("POST /v1/runs", authSubmit(http.HandlerFunc(s.submitRun)))
	mux.Handle("GET /v1/runs/{id}", authRead(http.HandlerFunc(s.getRun)))
	mux.Handle("GET /v1/runs", authRead(http.HandlerFunc(s.listRuns)))
	mux.Handle("POST /v1/runs/{id}/cancel", authCancel(http.HandlerFunc(s.cancelRun)))

	mux.Handle("POST /v1/webhooks", authWebhooks(http.HandlerFunc(s.createWebhook)))
	mux.Handle("GET /v1/webhooks", authWebhooks(http.HandlerFunc(s.listWebhooks)))
	mux.Handle("DELETE /v1/webhooks/{id}", authWebhooks(http.HandlerFunc(s.deleteWebhook)))

	mux.Handle("POST /v1/registry/definitions", authPublish(http.HandlerFunc(s.createDefinition)))
	mux.Handle("GET /v1/registry/definitions", authRead(http.HandlerFunc(s.listDefinitions)))
	mux.Handle("GET /v1/registry/definitions/{name}", authRead(http.HandlerFunc(s.getDefinition)))

	mux.Handle("POST /v1/migrate", authSubmit(http.HandlerFunc(s.migrate)))
	mux.Handle("GET /v1/readyz", http.HandlerFunc(s.readyz))
	mux.Handle("GET /v1/livez", http.HandlerFunc(s.livez))

	return http.Handler(RecoverPanic(AccessLog(mux)))
}

type sub struct {
	server *Server
}

func (s *sub) submitRun(w http.ResponseWriter, r *http.Request) {
	s.server.submitRun(w, r)
}

func (s *sub) getRun(w http.ResponseWriter, r *http.Request) {
	s.server.getRun(w, r)
}

func (s *sub) listRuns(w http.ResponseWriter, r *http.Request) {
	s.server.listRuns(w, r)
}

func (s *sub) cancelRun(w http.ResponseWriter, r *http.Request) {
	s.server.cancelRun(w, r)
}

func (s *sub) createWebhook(w http.ResponseWriter, r *http.Request) {
	s.server.createWebhook(w, r)
}

func (s *sub) listWebhooks(w http.ResponseWriter, r *http.Request) {
	s.server.listWebhooks(w, r)
}

func (s *sub) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	s.server.deleteWebhook(w, r)
}

func (s *sub) createDefinition(w http.ResponseWriter, r *http.Request) {
	s.server.createDefinition(w, r)
}

func (s *sub) listDefinitions(w http.ResponseWriter, r *http.Request) {
	s.server.listDefinitions(w, r)
}

func (s *sub) getDefinition(w http.ResponseWriter, r *http.Request) {
	s.server.getDefinition(w, r)
}

func (s *sub) migrate(w http.ResponseWriter, r *http.Request) {
	s.server.migrate(w, r)
}

func (s *sub) readyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func (s *sub) livez(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"alive"}`))
}

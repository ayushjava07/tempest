package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/tempest-io/tempest/internal/auth"
	"github.com/tempest-io/tempest/internal/persistence"
	"github.com/tempest-io/tempest/internal/scheduler"
	v1 "github.com/tempest-io/tempest/internal/api/v1"
	"github.com/tempest-io/tempest/pkg/errors"
	ttypes "github.com/tempest-io/tempest/pkg/types"
	"github.com/tempest-io/tempest/pkg/validation"
)

type Server struct {
	store    persistence.Store
	resolver *auth.Resolver
	engine   *scheduler.Engine
	http     *http.Server
}

type ServerOptions struct {
	Store    persistence.Store
	Resolver *auth.Resolver
	Engine   *scheduler.Engine
	Addr     string
}

func NewServer(opts ServerOptions) *Server {
	s := &Server{
		store:    opts.Store,
		resolver: opts.Resolver,
		engine:   opts.Engine,
	}
	s.http = &http.Server{
		Addr:              opts.Addr,
		Handler:           NewMux(s),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) submitRun(w http.ResponseWriter, r *http.Request) {
	var req v1.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		mapError(w, r, errors.Of(errors.ClassInvalidArgument, err))
		return
	}
	if err := validation.ValidateName(req.Workflow); err != nil {
		mapError(w, r, errors.Of(errors.ClassInvalidArgument, err))
		return
	}
	input := ttypes.RunInput{
		Workflow: ttypes.WorkflowID{Name: req.Workflow, Version: req.Version},
		Payload:  req.Input,
	}
	runID, err := s.engine.Submit(r.Context(), input)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(v1.SubmitResponse{RunID: runID})
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ns, _ := NamespaceFrom(r.Context())
	run, err := s.store.GetRun(r.Context(), ttypes.Namespace(ns), id)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v1.ToRun(run))
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	ns, _ := NamespaceFrom(r.Context())
	runs, err := s.store.ListRuns(r.Context(), persistence.RunFilter{
		Namespace: ttypes.Namespace(ns),
		Limit:     100,
	})
	if err != nil {
		mapError(w, r, err)
		return
	}
	items := make([]v1.Run, len(runs))
	for i := range runs {
		items[i] = v1.ToRun(&runs[i])
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v1.ListResponse[v1.Run]{Items: items, Total: len(items)})
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ns, _ := NamespaceFrom(r.Context())
	if err := s.engine.Cancel(r.Context(), ttypes.Namespace(ns), id); err != nil {
		mapError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createDefinition(w http.ResponseWriter, r *http.Request) {
	var def v1.WorkflowDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		mapError(w, r, errors.Of(errors.ClassInvalidArgument, err))
		return
	}
	steps := make([]ttypes.StepDefinition, len(def.Steps))
	for i, s := range def.Steps {
		dur, _ := time.ParseDuration(s.Timeout)
		steps[i] = ttypes.StepDefinition{
			ID:       s.ID,
			Handler:  s.Handler,
			DependsOn: s.Depends,
			Timeout:  dur,
			Retry:    ttypes.RetryPolicy{MaxAttempts: s.RetryMax},
		}
	}
	d := &ttypes.WorkflowDefinition{
		ID: ttypes.WorkflowID{
			Namespace: ttypes.Namespace(def.Namespace),
			Name:      def.Name,
			Version:   def.Version,
		},
		Steps:     steps,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.store.CreateDefinition(r.Context(), d); err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(v1.ToDefinition(d))
}

func (s *Server) listDefinitions(w http.ResponseWriter, r *http.Request) {
	defs, err := s.store.ListDefinitions(r.Context(), persistence.DefinitionFilter{})
	if err != nil {
		mapError(w, r, err)
		return
	}
	items := make([]v1.WorkflowDefinition, len(defs))
	for i := range defs {
		items[i] = v1.ToDefinition(&defs[i])
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v1.ListResponse[v1.WorkflowDefinition]{Items: items, Total: len(items)})
}

func (s *Server) getDefinition(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ns, _ := NamespaceFrom(r.Context())
	def, err := s.store.GetDefinitionByName(r.Context(), ttypes.Namespace(ns), name)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v1.ToDefinition(def))
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL    string `json:"url"`
		Secret string `json:"secret,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		mapError(w, r, errors.Of(errors.ClassInvalidArgument, err))
		return
	}
	ns, _ := NamespaceFrom(r.Context())
	ep := &ttypes.WebhookEndpoint{
		ID:        ttypes.NewID(),
		Namespace: ttypes.Namespace(ns),
		URL:       body.URL,
		Secret:    body.Secret,
		Active:    true,
		CreatedAt: time.Now(),
	}
	if err := s.store.CreateWebhookEndpoint(r.Context(), ep); err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": ep.ID, "url": ep.URL})
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	ns, _ := NamespaceFrom(r.Context())
	eps, err := s.store.ListWebhookEndpoints(r.Context(), ttypes.Namespace(ns))
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v1.ListResponse[ttypes.WebhookEndpoint]{Items: eps, Total: len(eps)})
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ns, _ := NamespaceFrom(r.Context())
	if err := s.store.DeleteWebhookEndpoint(r.Context(), ttypes.Namespace(ns), id); err != nil {
		mapError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) migrate(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

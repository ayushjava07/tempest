package plugin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tempest-io/tempest/pkg/errors"
)

type Handle struct {
	RunID     string
	StepID    string
	Namespace string
	RunInput  map[string]any
	Deadline  time.Duration
	Now       func() time.Time
}

type Result struct {
	Output map[string]any
	Error  error
}

func Errorf(format string, args ...any) Result {
	return Result{Error: fmt.Errorf(format, args...)}
}

type Handler interface {
	Name() string
	Description() string
	Execute(ctx context.Context, in Handle) (Result, error)
}

type HandlerSpec struct {
	Name        string
	Description string
}

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

func (r *Registry) Register(h Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := h.Name()
	if name == "" {
		return errors.InvalidArgumentError(fmt.Errorf("handler name must not be empty"))
	}
	if strings.ContainsAny(name, " \t\n") {
		return errors.InvalidArgumentError(fmt.Errorf("handler name %q must not contain whitespace", name))
	}
	if _, dup := r.handlers[name]; dup {
		return errors.AlreadyExistsError(fmt.Errorf("handler %q already registered", name))
	}
	r.handlers[name] = h
	return nil
}

func (r *Registry) Resolve(name string) (Handler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[name]
	if !ok {
		return nil, errors.NotFoundError(fmt.Errorf("no handler named %q", name))
	}
	return h, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) MustRegister(h Handler) {
	if err := r.Register(h); err != nil {
		panic(fmt.Sprintf("register handler %q: %v", h.Name(), err))
	}
}

func (r *Registry) Specs() []HandlerSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HandlerSpec, 0, len(r.handlers))
	for name, h := range r.handlers {
		out = append(out, HandlerSpec{Name: name, Description: describe(h)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[name]
	return ok
}

func describe(h Handler) string {
	d := h.Description()
	if d == "" {
		return h.Name()
	}
	return d
}

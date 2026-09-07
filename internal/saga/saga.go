package saga

import (
	"context"
	"fmt"
	"sync"
)

type Step struct {
	Name       string
	Execute    func(ctx context.Context) error
	Compensate func(ctx context.Context) error
}

type Saga struct {
	mu         sync.Mutex
	steps      []*Step
	executed   []*Step
	completed  bool
	failed     bool
	err        error
}

func New() *Saga {
	return &Saga{
		steps:    make([]*Step, 0),
		executed: make([]*Step, 0),
	}
}

func (s *Saga) AddStep(step *Step) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, step)
}

func (s *Saga) Execute(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed || s.failed {
		return fmt.Errorf("saga already finished")
	}
	for _, step := range s.steps {
		if err := step.Execute(ctx); err != nil {
			s.failed = true
			s.err = err
			s.compensate(ctx)
			return err
		}
		s.executed = append(s.executed, step)
	}
	s.completed = true
	return nil
}

func (s *Saga) compensate(ctx context.Context) {
	for i := len(s.executed) - 1; i >= 0; i-- {
		step := s.executed[i]
		if step.Compensate != nil {
			_ = step.Compensate(ctx)
		}
	}
}

func (s *Saga) IsCompleted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completed
}

func (s *Saga) IsFailed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}

func (s *Saga) Error() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Saga) StepsExecuted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.executed)
}

func (s *Saga) StepsTotal() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.steps)
}

type Orchestrator struct {
	mu     sync.Mutex
	sagas  map[string]*Saga
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		sagas: make(map[string]*Saga),
	}
}

func (o *Orchestrator) Create(id string) *Saga {
	o.mu.Lock()
	defer o.mu.Unlock()
	saga := New()
	o.sagas[id] = saga
	return saga
}

func (o *Orchestrator) Get(id string) (*Saga, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	s, ok := o.sagas[id]
	return s, ok
}

func (o *Orchestrator) Delete(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.sagas, id)
}

func (o *Orchestrator) List() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	ids := make([]string, 0, len(o.sagas))
	for id := range o.sagas {
		ids = append(ids, id)
	}
	return ids
}
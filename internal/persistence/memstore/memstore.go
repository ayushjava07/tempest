package memstore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
	"github.com/tempest-io/tempest/pkg/errors"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type Store struct {
	mu          sync.RWMutex
	definitions map[string]*ttypes.WorkflowDefinition
	runs        map[string]*ttypes.Run
	queue       map[string]*ttypes.QueueItem
	events      []*ttypes.Event
	webhooks    map[string]*ttypes.WebhookEndpoint
	deliveries  map[string]*ttypes.Delivery
	tokens      map[string]*ttypes.APIToken
	tokenHash   map[string]*ttypes.APIToken
	now         func() time.Time
}

func New() *Store {
	return &Store{
		definitions: make(map[string]*ttypes.WorkflowDefinition),
		runs:        make(map[string]*ttypes.Run),
		queue:       make(map[string]*ttypes.QueueItem),
		webhooks:    make(map[string]*ttypes.WebhookEndpoint),
		deliveries:  make(map[string]*ttypes.Delivery),
		tokens:      make(map[string]*ttypes.APIToken),
		tokenHash:   make(map[string]*ttypes.APIToken),
		now:         time.Now,
	}
}

func NewWithClock(clock func() time.Time) *Store {
	s := New()
	s.now = clock
	return s
}

func defKey(namespace ttypes.Namespace, name string, version int) string {
	return string(namespace) + "/" + name + "@" + itoa(version)
}

func runKey(namespace ttypes.Namespace, id string) string {
	return string(namespace) + "/" + id
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func (s *Store) CreateDefinition(_ context.Context, d *ttypes.WorkflowDefinition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(d.ID.Namespace, d.ID.Name, d.ID.Version)
	if _, ok := s.definitions[key]; ok {
		return errors.AlreadyExistsError(errors.ErrAlreadyExists)
	}
	s.definitions[key] = cloneDefinition(d)
	return nil
}

func (s *Store) GetDefinition(_ context.Context, id ttypes.WorkflowID) (*ttypes.WorkflowDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.definitions[defKey(id.Namespace, id.Name, id.Version)]
	if !ok {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	return cloneDefinition(d), nil
}

func (s *Store) GetDefinitionByName(_ context.Context, namespace ttypes.Namespace, name string) (*ttypes.WorkflowDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *ttypes.WorkflowDefinition
	for _, d := range s.definitions {
		if d.ID.Namespace == namespace && d.ID.Name == name {
			if latest == nil || d.ID.Version > latest.ID.Version {
				latest = d
			}
		}
	}
	if latest == nil {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	return cloneDefinition(latest), nil
}

func (s *Store) ListDefinitions(_ context.Context, f persistence.DefinitionFilter) ([]ttypes.WorkflowDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.WorkflowDefinition
	for _, d := range s.definitions {
		if f.Namespace != "" && d.ID.Namespace != f.Namespace {
			continue
		}
		if f.Name != "" && d.ID.Name != f.Name {
			continue
		}
		out = append(out, *cloneDefinition(d))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID.Name != out[j].ID.Name {
			return out[i].ID.Name < out[j].ID.Name
		}
		return out[i].ID.Version > out[j].ID.Version
	})
	if f.Offset > 0 {
		if f.Offset >= len(out) {
			return []ttypes.WorkflowDefinition{}, nil
		}
		out = out[f.Offset:]
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) DeleteDefinition(_ context.Context, id ttypes.WorkflowID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(id.Namespace, id.Name, id.Version)
	if _, ok := s.definitions[key]; !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	delete(s.definitions, key)
	return nil
}

func (s *Store) CreateRun(_ context.Context, r *ttypes.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runKey(r.Namespace, r.ID)
	if _, ok := s.runs[key]; ok {
		return errors.AlreadyExistsError(errors.ErrAlreadyExists)
	}
	s.runs[key] = cloneRun(r)
	return nil
}

func (s *Store) GetRun(_ context.Context, namespace ttypes.Namespace, id string) (*ttypes.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[runKey(namespace, id)]
	if !ok {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	return cloneRun(r), nil
}

func (s *Store) ListRuns(_ context.Context, f persistence.RunFilter) ([]ttypes.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.Run
	for _, r := range s.runs {
		if f.Namespace != "" && r.Namespace != f.Namespace {
			continue
		}
		if f.Workflow != "" && r.Workflow.Name != f.Workflow {
			continue
		}
		if f.State != nil && r.State != *f.State {
			continue
		}
		if f.CreatedAfter != nil && !r.CreatedAt.After(*f.CreatedAfter) {
			continue
		}
		out = append(out, cloneRunForList(r))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if f.Offset > 0 {
		if f.Offset >= len(out) {
			return []ttypes.Run{}, nil
		}
		out = out[f.Offset:]
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) UpdateRunState(_ context.Context, namespace ttypes.Namespace, id string, to ttypes.RunState, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runKey(namespace, id)]
	if !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	r.State = to
	switch {
	case r.StartedAt == nil && (to == ttypes.StateRunning || to == ttypes.StateSucceeded || to == ttypes.StateFailed):
		r.StartedAt = &at
	case to.IsTerminal():
		r.FinishedAt = &at
	}
	return nil
}

func (s *Store) UpdateRunError(_ context.Context, namespace ttypes.Namespace, id string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runKey(namespace, id)]
	if !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	r.Error = errMsg
	return nil
}

func (s *Store) UpdateStepState(_ context.Context, namespace ttypes.Namespace, id string, step ttypes.StepRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runKey(namespace, id)]
	if !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	for i, s := range r.Steps {
		if s.StepID == step.StepID {
			r.Steps[i] = step
			return nil
		}
	}
	return errors.NotFoundError(fmt.Errorf("step %q not found in run %q", step.StepID, id))
}

func (s *Store) Enqueue(_ context.Context, item ttypes.QueueItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(item.Namespace) + "/" + item.RunID + "/" + item.StepID
	cp := item
	s.queue[key] = &cp
	return nil
}

func (s *Store) Dequeue(_ context.Context, namespace ttypes.Namespace, limit int, now time.Time) ([]persistence.QueueLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []persistence.QueueLease
	for _, item := range s.queue {
		if item.Namespace != namespace {
			continue
		}
		if !item.VisibleAt.Before(now) {
			continue
		}
		item.Attempt++
		item.VisibleAt = now.Add(30 * time.Second)
		token := newLeaseToken()
		out = append(out, persistence.QueueLease{Item: *item, Token: token})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) DequeueAny(_ context.Context, limit int, now time.Time) ([]persistence.QueueLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []persistence.QueueLease
	for _, item := range s.queue {
		if !item.VisibleAt.Before(now) {
			continue
		}
		item.Attempt++
		item.VisibleAt = now.Add(30 * time.Second)
		token := newLeaseToken()
		out = append(out, persistence.QueueLease{Item: *item, Token: token})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) CompleteStep(_ context.Context, namespace ttypes.Namespace, runID, stepID, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(namespace) + "/" + runID + "/" + stepID
	if _, ok := s.queue[key]; !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	delete(s.queue, key)
	return nil
}

func (s *Store) ReleaseStep(_ context.Context, namespace ttypes.Namespace, runID, stepID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(namespace) + "/" + runID + "/" + stepID
	item, ok := s.queue[key]
	if !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	item.VisibleAt = s.now()
	return nil
}

func (s *Store) RequeueWithDelay(_ context.Context, namespace ttypes.Namespace, runID, stepID string, visibleAt time.Time, attempt int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(namespace) + "/" + runID + "/" + stepID
	item, ok := s.queue[key]
	if !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	item.VisibleAt = visibleAt
	item.Attempt = attempt
	return nil
}

func (s *Store) LeaseExpired(_ context.Context, leaseTimeout time.Duration, now time.Time) ([]ttypes.QueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.QueueItem
	for _, item := range s.queue {
		if item.Attempt > 0 && now.Sub(item.VisibleAt) > leaseTimeout {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (s *Store) ListQueue(_ context.Context, namespace ttypes.Namespace, limit int) ([]ttypes.QueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.QueueItem
	for _, item := range s.queue {
		if item.Namespace != namespace {
			continue
		}
		out = append(out, *item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) AppendEvent(_ context.Context, e *ttypes.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneEvent(e))
	return nil
}

func (s *Store) GetEvent(_ context.Context, id string) (*ttypes.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.events {
		if e.ID == id {
			return cloneEvent(e), nil
		}
	}
	return nil, errors.NotFoundError(errors.ErrNotFound)
}

func (s *Store) ListEvents(_ context.Context, f persistence.EventFilter) ([]ttypes.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.Event
	for _, e := range s.events {
		if f.Namespace != "" && e.Namespace != f.Namespace {
			continue
		}
		if f.RunID != "" && e.RunID != f.RunID {
			continue
		}
		if f.Type != "" && e.Type != f.Type {
			continue
		}
		out = append(out, *cloneEvent(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) CreateWebhookEndpoint(_ context.Context, e *ttypes.WebhookEndpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(e.Namespace, e.ID, 0)
	if _, ok := s.webhooks[key]; ok {
		return errors.AlreadyExistsError(errors.ErrAlreadyExists)
	}
	s.webhooks[key] = cloneEndpoint(e)
	return nil
}

func (s *Store) GetWebhookEndpoint(_ context.Context, namespace ttypes.Namespace, id string) (*ttypes.WebhookEndpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.webhooks[defKey(namespace, id, 0)]
	if !ok {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	return cloneEndpoint(e), nil
}

func (s *Store) ListWebhookEndpoints(_ context.Context, namespace ttypes.Namespace) ([]ttypes.WebhookEndpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.WebhookEndpoint
	for _, e := range s.webhooks {
		if e.Namespace == namespace {
			out = append(out, *cloneEndpoint(e))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) UpdateWebhookEndpoint(_ context.Context, e *ttypes.WebhookEndpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(e.Namespace, e.ID, 0)
	if _, ok := s.webhooks[key]; !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	s.webhooks[key] = cloneEndpoint(e)
	return nil
}

func (s *Store) DeleteWebhookEndpoint(_ context.Context, namespace ttypes.Namespace, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(namespace, id, 0)
	if _, ok := s.webhooks[key]; !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	delete(s.webhooks, key)
	return nil
}

func (s *Store) CreateDelivery(_ context.Context, d *ttypes.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(d.Namespace, d.ID, 0)
	if _, ok := s.deliveries[key]; ok {
		return errors.AlreadyExistsError(errors.ErrAlreadyExists)
	}
	s.deliveries[key] = cloneDelivery(d)
	return nil
}

func (s *Store) UpdateDelivery(_ context.Context, d *ttypes.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(d.Namespace, d.ID, 0)
	if _, ok := s.deliveries[key]; !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	s.deliveries[key] = cloneDelivery(d)
	return nil
}

func (s *Store) ListDeliveries(_ context.Context, f persistence.DeliveryFilter) ([]ttypes.Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.Delivery
	for _, d := range s.deliveries {
		if f.Namespace != "" && d.Namespace != f.Namespace {
			continue
		}
		if f.EndpointID != "" && d.EndpointID != f.EndpointID {
			continue
		}
		if f.Status != "" && d.Status != f.Status {
			continue
		}
		out = append(out, *cloneDelivery(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if f.Offset > 0 {
		if f.Offset >= len(out) {
			return []ttypes.Delivery{}, nil
		}
		out = out[f.Offset:]
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) CreateToken(_ context.Context, tok *ttypes.APIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(tok.Namespace, tok.ID, 0)
	if _, ok := s.tokens[key]; ok {
		return errors.AlreadyExistsError(errors.ErrAlreadyExists)
	}
	cp := cloneToken(tok)
	s.tokens[key] = cp
	if tok.TokenHash != "" {
		s.tokenHash[tok.TokenHash] = cp
	}
	return nil
}

func (s *Store) GetToken(_ context.Context, namespace ttypes.Namespace, id string) (*ttypes.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[defKey(namespace, id, 0)]
	if !ok {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	return cloneToken(t), nil
}

func (s *Store) LookupTokenByHash(_ context.Context, hash string) (*ttypes.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokenHash[hash]
	if !ok {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	return cloneToken(t), nil
}

func (s *Store) ListTokens(_ context.Context, f persistence.CredentialsFilter) ([]ttypes.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.APIToken
	for _, t := range s.tokens {
		if f.Namespace != "" && t.Namespace != f.Namespace {
			continue
		}
		out = append(out, *cloneToken(t))
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) DeleteToken(_ context.Context, namespace ttypes.Namespace, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := defKey(namespace, id, 0)
	t, ok := s.tokens[key]
	if !ok {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	delete(s.tokens, key)
	if t.TokenHash != "" {
		delete(s.tokenHash, t.TokenHash)
	}
	return nil
}

func (s *Store) ListExpiredRuns(_ context.Context, cutoff time.Time, limit int) ([]ttypes.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ttypes.Run
	for _, r := range s.runs {
		if r.FinishedAt == nil || !r.FinishedAt.Before(cutoff) {
			continue
		}
		if !r.State.IsTerminal() {
			continue
		}
		out = append(out, *cloneRun(r))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FinishedAt.Before(*out[j].FinishedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) DeleteRuns(_ context.Context, ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for _, id := range ids {
		for qk, qr := range s.runs {
			if qr.ID == id {
				delete(s.runs, qk)
				deleted++
			}
		}
		for qk := range s.queue {
			if strings.Contains(qk, "/"+id+"/") {
				delete(s.queue, qk)
			}
		}
	}
	return deleted, nil
}

func (s *Store) CompactRuns(_ context.Context, before time.Time, limit int) (int, error) {
	expired, err := s.ListExpiredRuns(context.Background(), before, limit)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(expired))
	for _, r := range expired {
		ids = append(ids, r.ID)
	}
	return s.DeleteRuns(context.Background(), ids)
}

func (s *Store) Close() error { return nil }

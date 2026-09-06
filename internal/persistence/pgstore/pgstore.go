package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tempest-io/tempest/internal/persistence"
	ttypes "github.com/tempest-io/tempest/pkg/types"
	"github.com/tempest-io/tempest/pkg/errors"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func Connect(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return New(pool), nil
}

func (s *Store) Close() error { s.pool.Close(); return nil }

func (s *Store) CreateDefinition(ctx context.Context, d *ttypes.WorkflowDefinition) error {
	steps, _ := json.Marshal(d.Steps)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_definitions (namespace, name, version, steps, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (namespace, name, version) DO UPDATE SET steps=$4, updated_at=$6`,
		string(d.ID.Namespace), d.ID.Name, d.ID.Version, steps, d.CreatedAt, time.Now(),
	)
	if err != nil {
		return errors.InternalError(fmt.Errorf("create definition: %w", err))
	}
	return nil
}

func (s *Store) GetDefinition(ctx context.Context, id ttypes.WorkflowID) (*ttypes.WorkflowDefinition, error) {
	return s.GetDefinitionByName(ctx, id.Namespace, id.Name)
}

func (s *Store) GetDefinitionByName(ctx context.Context, namespace ttypes.Namespace, name string) (*ttypes.WorkflowDefinition, error) {
	var ns, n string
	var ver int
	var steps []byte
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT namespace, name, version, steps, created_at, updated_at
		 FROM workflow_definitions WHERE namespace=$1 AND name=$2 ORDER BY version DESC LIMIT 1`, string(namespace), name,
	).Scan(&ns, &n, &ver, &steps, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	if err != nil {
		return nil, errors.InternalError(fmt.Errorf("get definition: %w", err))
	}
	var stepDefs []ttypes.StepDefinition
	_ = json.Unmarshal(steps, &stepDefs)
	return &ttypes.WorkflowDefinition{
		ID:        ttypes.WorkflowID{Namespace: ttypes.Namespace(ns), Name: n, Version: ver},
		Steps:     stepDefs,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func (s *Store) ListDefinitions(ctx context.Context, f persistence.DefinitionFilter) ([]ttypes.WorkflowDefinition, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT namespace, name, version, steps, created_at, updated_at
		 FROM workflow_definitions ORDER BY name, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.WorkflowDefinition
	for rows.Next() {
		var ns, n string
		var ver int
		var steps []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&ns, &n, &ver, &steps, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var stepDefs []ttypes.StepDefinition
		_ = json.Unmarshal(steps, &stepDefs)
		out = append(out, ttypes.WorkflowDefinition{
			ID:        ttypes.WorkflowID{Namespace: ttypes.Namespace(ns), Name: n, Version: ver},
			Steps:     stepDefs,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	return out, nil
}

func (s *Store) DeleteDefinition(ctx context.Context, id ttypes.WorkflowID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM workflow_definitions WHERE namespace=$1 AND name=$2 AND version=$3`,
		string(id.Namespace), id.Name, id.Version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	return nil
}

func (s *Store) CreateRun(ctx context.Context, run *ttypes.Run) error {
	input, _ := json.Marshal(run.Input)
	steps, _ := json.Marshal(run.Steps)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO runs (namespace, id, workflow_name, workflow_version, state, input, steps, error, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		string(run.Namespace), run.ID, run.Workflow.Name, run.Workflow.Version,
		string(run.State), input, steps, run.Error, run.CreatedAt,
	)
	if err != nil {
		return errors.InternalError(fmt.Errorf("create run: %w", err))
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, namespace ttypes.Namespace, id string) (*ttypes.Run, error) {
	var ns, rid, state, wfName, errMsg string
	var wfVer int
	var input, steps []byte
	var createdAt time.Time
	var startedAt, finishedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT namespace, id, workflow_name, workflow_version, state, input, steps, COALESCE(error,''), created_at, started_at, finished_at
		 FROM runs WHERE namespace=$1 AND id=$2`, string(namespace), id,
	).Scan(&ns, &rid, &wfName, &wfVer, &state, &input, &steps, &errMsg, &createdAt, &startedAt, &finishedAt)
	if err == pgx.ErrNoRows {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	if err != nil {
		return nil, errors.InternalError(fmt.Errorf("get run: %w", err))
	}
	var runInput ttypes.RunInput
	var stepRuns []ttypes.StepRun
	_ = json.Unmarshal(input, &runInput)
	_ = json.Unmarshal(steps, &stepRuns)
	return &ttypes.Run{
		ID: rid, Namespace: ttypes.Namespace(ns),
		Workflow:  ttypes.WorkflowID{Name: wfName, Version: wfVer},
		State:     ttypes.RunState(state),
		Input:     runInput,
		Steps:     stepRuns,
		Error:     errMsg,
		CreatedAt: createdAt,
		StartedAt: startedAt,
		FinishedAt: finishedAt,
	}, nil
}

func (s *Store) ListRuns(ctx context.Context, f persistence.RunFilter) ([]ttypes.Run, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT namespace, id, workflow_name, workflow_version, state, input, steps, COALESCE(error,''), created_at, started_at, finished_at
		 FROM runs WHERE namespace=$1 ORDER BY created_at DESC LIMIT $2`, string(f.Namespace), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func scanRuns(rows pgx.Rows) ([]ttypes.Run, error) {
	var out []ttypes.Run
	for rows.Next() {
		var ns, rid, state, wfName, errMsg string
		var wfVer int
		var input, steps []byte
		var createdAt time.Time
		var startedAt, finishedAt *time.Time
		if err := rows.Scan(&ns, &rid, &wfName, &wfVer, &state, &input, &steps, &errMsg, &createdAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		var runInput ttypes.RunInput
		var stepRuns []ttypes.StepRun
		_ = json.Unmarshal(input, &runInput)
		_ = json.Unmarshal(steps, &stepRuns)
		out = append(out, ttypes.Run{
			ID: rid, Namespace: ttypes.Namespace(ns),
			Workflow:  ttypes.WorkflowID{Name: wfName, Version: wfVer},
			State:     ttypes.RunState(state),
			Input:     runInput,
			Steps:     stepRuns,
			Error:     errMsg,
			CreatedAt: createdAt,
			StartedAt: startedAt,
			FinishedAt: finishedAt,
		})
	}
	return out, nil
}

func (s *Store) UpdateRunState(ctx context.Context, namespace ttypes.Namespace, id string, state ttypes.RunState, at time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET state=$1 WHERE namespace=$2 AND id=$3`,
		string(state), string(namespace), id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	return nil
}

func (s *Store) UpdateRunError(ctx context.Context, namespace ttypes.Namespace, id, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE runs SET error=$1 WHERE namespace=$2 AND id=$3`,
		errMsg, string(namespace), id,
	)
	return err
}

func (s *Store) UpdateStepState(ctx context.Context, namespace ttypes.Namespace, id string, step ttypes.StepRun) error {
	run, err := s.GetRun(ctx, namespace, id)
	if err != nil {
		return err
	}
	for i, s := range run.Steps {
		if s.StepID == step.StepID {
			run.Steps[i] = step
			break
		}
	}
	steps, _ := json.Marshal(run.Steps)
	_, err = s.pool.Exec(ctx,
		`UPDATE runs SET steps=$1 WHERE namespace=$2 AND id=$3`,
		steps, string(namespace), id,
	)
	return err
}

func (s *Store) Enqueue(ctx context.Context, item ttypes.QueueItem) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO queue (namespace, run_id, step_id, enqueued_at, visible_at, attempt)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		string(item.Namespace), item.RunID, item.StepID, item.EnqueuedAt, item.VisibleAt, item.Attempt,
	)
	return err
}

func (s *Store) Dequeue(ctx context.Context, namespace ttypes.Namespace, limit int, now time.Time) ([]persistence.QueueLease, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx,
		`UPDATE queue SET attempt=attempt+1, visible_at=$1
		 WHERE ctid IN (
		   SELECT ctid FROM queue WHERE namespace=$2 AND visible_at < $3
		   ORDER BY visible_at ASC LIMIT $4 FOR UPDATE SKIP LOCKED
		 ) RETURNING namespace, run_id, step_id, enqueued_at, visible_at, attempt`,
		now.Add(30*time.Second), string(namespace), now, limit,
	)
	if err != nil {
		return nil, err
	}
	var out []persistence.QueueLease
	for rows.Next() {
		var ns, runID, stepID string
		var enqueuedAt, visibleAt time.Time
		var attempt int
		if err := rows.Scan(&ns, &runID, &stepID, &enqueuedAt, &visibleAt, &attempt); err != nil {
			return nil, err
		}
		out = append(out, persistence.QueueLease{
			Item: ttypes.QueueItem{
				Namespace: ttypes.Namespace(ns), RunID: runID, StepID: stepID,
				EnqueuedAt: enqueuedAt, VisibleAt: visibleAt, Attempt: attempt,
			},
			Token: persistence.NewLeaseToken(),
		})
	}
	return out, tx.Commit(ctx)
}

func (s *Store) DequeueAny(ctx context.Context, limit int, now time.Time) ([]persistence.QueueLease, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx,
		`UPDATE queue SET attempt=attempt+1, visible_at=$1
		 WHERE ctid IN (
		   SELECT ctid FROM queue WHERE visible_at < $2
		   ORDER BY visible_at ASC LIMIT $3 FOR UPDATE SKIP LOCKED
		 ) RETURNING namespace, run_id, step_id, enqueued_at, visible_at, attempt`,
		now.Add(30*time.Second), now, limit,
	)
	if err != nil {
		return nil, err
	}
	var out []persistence.QueueLease
	for rows.Next() {
		var ns, runID, stepID string
		var enqueuedAt, visibleAt time.Time
		var attempt int
		if err := rows.Scan(&ns, &runID, &stepID, &enqueuedAt, &visibleAt, &attempt); err != nil {
			return nil, err
		}
		out = append(out, persistence.QueueLease{
			Item: ttypes.QueueItem{
				Namespace: ttypes.Namespace(ns), RunID: runID, StepID: stepID,
				EnqueuedAt: enqueuedAt, VisibleAt: visibleAt, Attempt: attempt,
			},
			Token: persistence.NewLeaseToken(),
		})
	}
	return out, tx.Commit(ctx)
}

func (s *Store) CompleteStep(ctx context.Context, namespace ttypes.Namespace, runID, stepID, leaseToken string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM queue WHERE namespace=$1 AND run_id=$2 AND step_id=$3`,
		string(namespace), runID, stepID,
	)
	return err
}

func (s *Store) ReleaseStep(ctx context.Context, namespace ttypes.Namespace, runID, stepID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE queue SET visible_at=now() WHERE namespace=$1 AND run_id=$2 AND step_id=$3`,
		string(namespace), runID, stepID,
	)
	return err
}

func (s *Store) RequeueWithDelay(ctx context.Context, namespace ttypes.Namespace, runID, stepID string, visibleAt time.Time, attempt int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE queue SET visible_at=$1, attempt=$2 WHERE namespace=$3 AND run_id=$4 AND step_id=$5`,
		visibleAt, attempt, string(namespace), runID, stepID,
	)
	return err
}

func (s *Store) LeaseExpired(ctx context.Context, leaseTimeout time.Duration, now time.Time) ([]ttypes.QueueItem, error) {
	cutoff := now.Add(-leaseTimeout)
	rows, err := s.pool.Query(ctx,
		`SELECT namespace, run_id, step_id, enqueued_at, visible_at, attempt
		 FROM queue WHERE attempt > 0 AND visible_at < $1`, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.QueueItem
	for rows.Next() {
		var item ttypes.QueueItem
		if err := rows.Scan(&item.Namespace, &item.RunID, &item.StepID, &item.EnqueuedAt, &item.VisibleAt, &item.Attempt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ListQueue(ctx context.Context, namespace ttypes.Namespace, limit int) ([]ttypes.QueueItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT namespace, run_id, step_id, enqueued_at, visible_at, attempt
		 FROM queue WHERE namespace=$1 ORDER BY enqueued_at ASC LIMIT $2`, string(namespace), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.QueueItem
	for rows.Next() {
		var item ttypes.QueueItem
		if err := rows.Scan(&item.Namespace, &item.RunID, &item.StepID, &item.EnqueuedAt, &item.VisibleAt, &item.Attempt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) AppendEvent(ctx context.Context, ev *ttypes.Event) error {
	payload, _ := json.Marshal(ev.Payload)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events (id, type, version, namespace, run_id, step_id, created_at, attempt, payload)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		ev.ID, string(ev.Type), ev.Version, string(ev.Namespace),
		ev.RunID, ev.StepID, ev.CreatedAt, ev.Attempt, payload,
	)
	return err
}

func (s *Store) GetEvent(ctx context.Context, id string) (*ttypes.Event, error) {
	var eid, typ, ns, runID, stepID string
	var ver int
	var createdAt time.Time
	var attempt int
	var payload []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, type, version, namespace, run_id, step_id, created_at, attempt, payload
		 FROM events WHERE id=$1`, id,
	).Scan(&eid, &typ, &ver, &ns, &runID, &stepID, &createdAt, &attempt, &payload)
	if err == pgx.ErrNoRows {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var p map[string]any
	_ = json.Unmarshal(payload, &p)
	return &ttypes.Event{
		ID: eid, Type: ttypes.EventType(typ), Version: ver,
		Namespace: ttypes.Namespace(ns), RunID: runID, StepID: stepID,
		CreatedAt: createdAt, Attempt: attempt, Payload: p,
	}, nil
}

func (s *Store) ListEvents(ctx context.Context, f persistence.EventFilter) ([]ttypes.Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, type, version, namespace, run_id, step_id, created_at, attempt, payload
		 FROM events WHERE namespace=$1 ORDER BY created_at ASC`, string(f.Namespace),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.Event
	for rows.Next() {
		var eid, typ, ns, runID, stepID string
		var ver int
		var createdAt time.Time
		var attempt int
		var payload []byte
		if err := rows.Scan(&eid, &typ, &ver, &ns, &runID, &stepID, &createdAt, &attempt, &payload); err != nil {
			return nil, err
		}
		var p map[string]any
		_ = json.Unmarshal(payload, &p)
		out = append(out, ttypes.Event{
			ID: eid, Type: ttypes.EventType(typ), Version: ver,
			Namespace: ttypes.Namespace(ns), RunID: runID, StepID: stepID,
			CreatedAt: createdAt, Attempt: attempt, Payload: p,
		})
	}
	return out, nil
}

func (s *Store) CreateWebhookEndpoint(ctx context.Context, wh *ttypes.WebhookEndpoint) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO webhooks (id, namespace, url, secret, types, active, timeout, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		wh.ID, string(wh.Namespace), wh.URL, wh.Secret,
		wh.Types, wh.Active, wh.Timeout, wh.CreatedAt,
	)
	return err
}

func (s *Store) GetWebhookEndpoint(ctx context.Context, namespace ttypes.Namespace, id string) (*ttypes.WebhookEndpoint, error) {
	var ns, wid, url, secret string
	var types []ttypes.EventType
	var active bool
	var timeout time.Duration
	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, namespace, url, secret, types, active, timeout, created_at
		 FROM webhooks WHERE namespace=$1 AND id=$2`, string(namespace), id,
	).Scan(&wid, &ns, &url, &secret, &types, &active, &timeout, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &ttypes.WebhookEndpoint{
		ID: wid, Namespace: ttypes.Namespace(ns), URL: url,
		Secret: secret, Types: types, Active: active,
		Timeout: timeout, CreatedAt: createdAt,
	}, nil
}

func (s *Store) ListWebhookEndpoints(ctx context.Context, namespace ttypes.Namespace) ([]ttypes.WebhookEndpoint, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, namespace, url, secret, types, active, timeout, created_at
		 FROM webhooks WHERE namespace=$1 ORDER BY created_at DESC`, string(namespace),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.WebhookEndpoint
	for rows.Next() {
		var wh ttypes.WebhookEndpoint
		var ns string
		if err := rows.Scan(&wh.ID, &ns, &wh.URL, &wh.Secret, &wh.Types, &wh.Active, &wh.Timeout, &wh.CreatedAt); err != nil {
			return nil, err
		}
		wh.Namespace = ttypes.Namespace(ns)
		out = append(out, wh)
	}
	return out, nil
}

func (s *Store) UpdateWebhookEndpoint(ctx context.Context, wh *ttypes.WebhookEndpoint) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE webhooks SET url=$1, secret=$2, types=$3, active=$4, timeout=$5
		 WHERE namespace=$6 AND id=$7`,
		wh.URL, wh.Secret, wh.Types, wh.Active, wh.Timeout,
		string(wh.Namespace), wh.ID,
	)
	return err
}

func (s *Store) DeleteWebhookEndpoint(ctx context.Context, namespace ttypes.Namespace, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM webhooks WHERE namespace=$1 AND id=$2`, string(namespace), id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	return nil
}

func (s *Store) CreateDelivery(ctx context.Context, d *ttypes.Delivery) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO deliveries (id, event_id, endpoint_id, namespace, status, attempts, last_status, last_error, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		d.ID, d.EventID, d.EndpointID, string(d.Namespace), string(d.Status),
		d.Attempts, d.LastStatus, d.LastError, d.CreatedAt,
	)
	return err
}

func (s *Store) UpdateDelivery(ctx context.Context, d *ttypes.Delivery) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE deliveries SET status=$1, attempts=$2, last_status=$3, last_error=$4, attempted_at=$5, finished_at=$6
		 WHERE id=$7`,
		string(d.Status), d.Attempts, d.LastStatus, d.LastError,
		d.AttemptedAt, d.FinishedAt, d.ID,
	)
	return err
}

func (s *Store) ListDeliveries(ctx context.Context, f persistence.DeliveryFilter) ([]ttypes.Delivery, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, event_id, endpoint_id, namespace, status, attempts, last_status, COALESCE(last_error,''), created_at, attempted_at, finished_at
		 FROM deliveries WHERE namespace=$1 ORDER BY created_at ASC LIMIT $2`, string(f.Namespace), f.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.Delivery
	for rows.Next() {
		var d ttypes.Delivery
		var ns string
		if err := rows.Scan(&d.ID, &d.EventID, &d.EndpointID, &ns, &d.Status, &d.Attempts, &d.LastStatus, &d.LastError, &d.CreatedAt, &d.AttemptedAt, &d.FinishedAt); err != nil {
			return nil, err
		}
		d.Namespace = ttypes.Namespace(ns)
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) CreateToken(ctx context.Context, tok *ttypes.APIToken) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_tokens (id, namespace, role, label, token_hash, created_at, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		tok.ID, string(tok.Namespace), tok.Role, tok.Label, tok.TokenHash, tok.CreatedAt, tok.ExpiresAt,
	)
	return err
}

func (s *Store) GetToken(ctx context.Context, namespace ttypes.Namespace, id string) (*ttypes.APIToken, error) {
	var tid, ns, role, label, hash string
	var createdAt time.Time
	var expiresAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, namespace, role, label, token_hash, created_at, expires_at
		 FROM api_tokens WHERE namespace=$1 AND id=$2`, string(namespace), id,
	).Scan(&tid, &ns, &role, &label, &hash, &createdAt, &expiresAt)
	if err == pgx.ErrNoRows {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &ttypes.APIToken{
		ID: tid, Namespace: ttypes.Namespace(ns), Role: role,
		Label: label, TokenHash: hash, CreatedAt: createdAt, ExpiresAt: expiresAt,
	}, nil
}

func (s *Store) LookupTokenByHash(ctx context.Context, hash string) (*ttypes.APIToken, error) {
	var tid, ns, role, label, tokenHash string
	var createdAt time.Time
	var expiresAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, namespace, role, label, token_hash, created_at, expires_at
		 FROM api_tokens WHERE token_hash=$1`, hash,
	).Scan(&tid, &ns, &role, &label, &tokenHash, &createdAt, &expiresAt)
	if err == pgx.ErrNoRows {
		return nil, errors.NotFoundError(errors.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &ttypes.APIToken{
		ID: tid, Namespace: ttypes.Namespace(ns), Role: role,
		Label: label, TokenHash: tokenHash, CreatedAt: createdAt, ExpiresAt: expiresAt,
	}, nil
}

func (s *Store) ListTokens(ctx context.Context, f persistence.CredentialsFilter) ([]ttypes.APIToken, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, namespace, role, label, token_hash, created_at, expires_at
		 FROM api_tokens WHERE namespace=$1 ORDER BY created_at DESC`, string(f.Namespace),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ttypes.APIToken
	for rows.Next() {
		var t ttypes.APIToken
		var ns string
		if err := rows.Scan(&t.ID, &ns, &t.Role, &t.Label, &t.TokenHash, &t.CreatedAt, &t.ExpiresAt); err != nil {
			return nil, err
		}
		t.Namespace = ttypes.Namespace(ns)
		out = append(out, t)
	}
	return out, nil
}

func (s *Store) DeleteToken(ctx context.Context, namespace ttypes.Namespace, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM api_tokens WHERE namespace=$1 AND id=$2`, string(namespace), id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFoundError(errors.ErrNotFound)
	}
	return nil
}

func (s *Store) ListExpiredRuns(ctx context.Context, cutoff time.Time, limit int) ([]ttypes.Run, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT namespace, id, workflow_name, workflow_version, state, input, steps, COALESCE(error,''), created_at, started_at, finished_at
		 FROM runs WHERE state IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT') AND finished_at < $1
		 ORDER BY finished_at ASC LIMIT $2`, cutoff, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (s *Store) DeleteRuns(ctx context.Context, ids []string) (int, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM runs WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CompactRuns(ctx context.Context, before time.Time, limit int) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM runs WHERE state IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT') AND finished_at < $1
		 AND id IN (SELECT id FROM runs WHERE state IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT') AND finished_at < $1 ORDER BY finished_at ASC LIMIT $2)`,
		before, limit,
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

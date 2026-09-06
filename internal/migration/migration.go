package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNilExecutor           = errors.New("migration: executor is nil")
	ErrDuplicateVersion      = errors.New("migration: duplicate migration version")
	ErrNonMonotonicVersion   = errors.New("migration: migration versions must be strictly increasing")
	ErrChecksumMismatch      = errors.New("migration: checksum mismatch for already applied migration")
	ErrLockAcquisitionFailed = errors.New("migration: failed to acquire distributed migration lock")
	ErrEmptyMigrationName    = errors.New("migration: migration name cannot be empty")
	ErrEmptyUpSQL            = errors.New("migration: migration Up SQL cannot be empty")
)

const DefaultAdvisoryLockID int64 = 0x54454D50455354 // "TEMPEST" in hex

// Migration defines a single database migration step with forward and reverse DDL.
type Migration struct {
	Version  int
	Name     string
	Up       string
	Down     string
	Checksum string
}

// MigrationRecord represents an applied migration in the tracking table.
type MigrationRecord struct {
	Version         int
	Name            string
	Checksum        string
	AppliedAt       time.Time
	ExecutionTimeMs int64
}

// MigrationStatus reports the current application state of a registered migration.
type MigrationStatus struct {
	Version         int
	Name            string
	Checksum        string
	Applied         bool
	AppliedAt       *time.Time
	ExecutionTimeMs int64
}

// Executor abstracts database interaction so migrations can run against pgxpool or test harnesses.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	QueryRow(ctx context.Context, sql string, args ...any) (RowScanner, error)
	Query(ctx context.Context, sql string, args ...any) (RowsScanner, error)
	AcquireLock(ctx context.Context, lockID int64) error
	ReleaseLock(ctx context.Context, lockID int64) error
}

// RowScanner abstracts scanning a single row.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner abstracts iterating and scanning multiple rows.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// PgxExecutor adapts *pgxpool.Pool to Executor with PostgreSQL advisory locks.
type PgxExecutor struct {
	pool *pgxpool.Pool
}

func NewPgxExecutor(pool *pgxpool.Pool) *PgxExecutor {
	return &PgxExecutor{pool: pool}
}

func (p *PgxExecutor) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (p *PgxExecutor) QueryRow(ctx context.Context, sql string, args ...any) (RowScanner, error) {
	return p.pool.QueryRow(ctx, sql, args...), nil
}

func (p *PgxExecutor) Query(ctx context.Context, sql string, args ...any) (RowsScanner, error) {
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRowsWrapper{rows: rows}, nil
}

func (p *PgxExecutor) AcquireLock(ctx context.Context, lockID int64) error {
	var locked bool
	err := p.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&locked)
	if err != nil {
		return fmt.Errorf("pg_try_advisory_lock: %w", err)
	}
	if !locked {
		return ErrLockAcquisitionFailed
	}
	return nil
}

func (p *PgxExecutor) ReleaseLock(ctx context.Context, lockID int64) error {
	var unlocked bool
	err := p.pool.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", lockID).Scan(&unlocked)
	if err != nil {
		return fmt.Errorf("pg_advisory_unlock: %w", err)
	}
	return nil
}

type pgxRowsWrapper struct {
	rows pgx.Rows
}

func (w *pgxRowsWrapper) Next() bool {
	return w.rows.Next()
}

func (w *pgxRowsWrapper) Scan(dest ...any) error {
	return w.rows.Scan(dest...)
}

func (w *pgxRowsWrapper) Close() {
	w.rows.Close()
}

func (w *pgxRowsWrapper) Err() error {
	return w.rows.Err()
}

// ComputeChecksum calculates standard SHA-256 hash of the normalized migration script.
func ComputeChecksum(sql string) string {
	normalized := strings.TrimSpace(sql)
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// Migrator manages and executes database migrations.
type Migrator struct {
	exec       Executor
	lockID     int64
	migrations []Migration
	mu         sync.RWMutex
}

// New creates a Migrator with the default PostgreSQL pool.
func New(pool *pgxpool.Pool) *Migrator {
	return NewWithExecutor(NewPgxExecutor(pool))
}

// NewWithExecutor creates a Migrator with a custom Executor implementation.
func NewWithExecutor(exec Executor) *Migrator {
	m := &Migrator{
		exec:       exec,
		lockID:     DefaultAdvisoryLockID,
		migrations: make([]Migration, 0),
	}
	m.registerDefaultMigrations()
	return m
}

// SetLockID configures the advisory lock ID used during migration synchronization.
func (m *Migrator) SetLockID(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockID = id
}

// Register adds one or more migrations, validating versions and calculating checksums.
func (m *Migrator) Register(migrations ...Migration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mig := range migrations {
		if mig.Name == "" {
			return ErrEmptyMigrationName
		}
		if mig.Up == "" {
			return ErrEmptyUpSQL
		}
		if len(m.migrations) > 0 {
			last := m.migrations[len(m.migrations)-1]
			if mig.Version == last.Version {
				return fmt.Errorf("%w: version %d", ErrDuplicateVersion, mig.Version)
			}
			if mig.Version < last.Version {
				return fmt.Errorf("%w: version %d after %d", ErrNonMonotonicVersion, mig.Version, last.Version)
			}
		}
		if mig.Checksum == "" {
			mig.Checksum = ComputeChecksum(mig.Up)
		}
		m.migrations = append(m.migrations, mig)
	}
	return nil
}

// Migrate executes all pending migrations within an advisory lock.
func (m *Migrator) Migrate(ctx context.Context) error {
	if m.exec == nil {
		return ErrNilExecutor
	}

	if err := m.exec.AcquireLock(ctx, m.lockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_ = m.exec.ReleaseLock(ctx, m.lockID)
	}()

	// 1. Ensure tracking table exists first (resolves bootstrap chicken-and-egg defect)
	if err := m.ensureMigrationTable(ctx); err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}

	// 2. Load already applied migrations
	applied, err := m.loadAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("load applied migrations: %w", err)
	}

	m.mu.RLock()
	migrations := append([]Migration(nil), m.migrations...)
	m.mu.RUnlock()

	// 3. Verify integrity of already applied migrations
	for _, mig := range migrations {
		if rec, exists := applied[mig.Version]; exists {
			expectedChecksum := mig.Checksum
			if expectedChecksum == "" {
				expectedChecksum = ComputeChecksum(mig.Up)
			}
			if rec.Checksum != "" && rec.Checksum != expectedChecksum {
				return fmt.Errorf("%w: migration %d (%s) expected %s, got %s",
					ErrChecksumMismatch, mig.Version, mig.Name, expectedChecksum, rec.Checksum)
			}
		}
	}

	// 4. Apply unapplied migrations in ascending version order
	for _, mig := range migrations {
		if _, exists := applied[mig.Version]; exists {
			continue
		}

		start := time.Now()
		if _, err := m.exec.Exec(ctx, mig.Up); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", mig.Version, mig.Name, err)
		}
		durationMs := time.Since(start).Milliseconds()

		checksum := mig.Checksum
		if checksum == "" {
			checksum = ComputeChecksum(mig.Up)
		}

		insertSQL := `INSERT INTO schema_migrations (version, name, checksum, applied_at, execution_time_ms) VALUES ($1, $2, $3, $4, $5)`
		if _, err := m.exec.Exec(ctx, insertSQL, mig.Version, mig.Name, checksum, time.Now().UTC(), durationMs); err != nil {
			return fmt.Errorf("record migration %d (%s): %w", mig.Version, mig.Name, err)
		}
	}

	return nil
}

// Rollback rolls back the last n applied migrations in descending order.
func (m *Migrator) Rollback(ctx context.Context, steps int) (int, error) {
	if m.exec == nil {
		return 0, ErrNilExecutor
	}
	if steps <= 0 {
		return 0, nil
	}

	if err := m.exec.AcquireLock(ctx, m.lockID); err != nil {
		return 0, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_ = m.exec.ReleaseLock(ctx, m.lockID)
	}()

	if err := m.ensureMigrationTable(ctx); err != nil {
		return 0, err
	}

	applied, err := m.loadAppliedMigrations(ctx)
	if err != nil {
		return 0, err
	}

	m.mu.RLock()
	migMap := make(map[int]Migration, len(m.migrations))
	for _, mig := range m.migrations {
		migMap[mig.Version] = mig
	}
	m.mu.RUnlock()

	// Find applied versions in descending order
	var appliedVersions []int
	for v := range applied {
		appliedVersions = append(appliedVersions, v)
	}
	// Sort descending
	for i := 0; i < len(appliedVersions); i++ {
		for j := i + 1; j < len(appliedVersions); j++ {
			if appliedVersions[i] < appliedVersions[j] {
				appliedVersions[i], appliedVersions[j] = appliedVersions[j], appliedVersions[i]
			}
		}
	}

	rolledBack := 0
	for _, v := range appliedVersions {
		if rolledBack >= steps {
			break
		}
		mig, ok := migMap[v]
		if !ok {
			return rolledBack, fmt.Errorf("migration version %d not found in registered migrations", v)
		}
		if mig.Down == "" {
			return rolledBack, fmt.Errorf("migration %d (%s) has no Down SQL definition", mig.Version, mig.Name)
		}

		if _, err := m.exec.Exec(ctx, mig.Down); err != nil {
			return rolledBack, fmt.Errorf("rollback migration %d (%s): %w", mig.Version, mig.Name, err)
		}

		deleteSQL := `DELETE FROM schema_migrations WHERE version = $1`
		if _, err := m.exec.Exec(ctx, deleteSQL, mig.Version); err != nil {
			return rolledBack, fmt.Errorf("delete migration record %d (%s): %w", mig.Version, mig.Name, err)
		}
		rolledBack++
	}

	return rolledBack, nil
}

// Status inspects the current migration status.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	if m.exec == nil {
		return nil, ErrNilExecutor
	}

	if err := m.ensureMigrationTable(ctx); err != nil {
		return nil, err
	}

	applied, err := m.loadAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]MigrationStatus, 0, len(m.migrations))
	for _, mig := range m.migrations {
		st := MigrationStatus{
			Version:  mig.Version,
			Name:     mig.Name,
			Checksum: mig.Checksum,
		}
		if rec, ok := applied[mig.Version]; ok {
			st.Applied = true
			appliedTime := rec.AppliedAt
			st.AppliedAt = &appliedTime
			st.ExecutionTimeMs = rec.ExecutionTimeMs
		}
		statuses = append(statuses, st)
	}

	return statuses, nil
}

func (m *Migrator) ensureMigrationTable(ctx context.Context) error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT NOT NULL,
			name TEXT NOT NULL,
			checksum TEXT NOT NULL DEFAULT '',
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			execution_time_ms BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (version)
		)`
	_, err := m.exec.Exec(ctx, createTableSQL)
	return err
}

func (m *Migrator) loadAppliedMigrations(ctx context.Context) (map[int]MigrationRecord, error) {
	querySQL := `SELECT version, name, checksum, applied_at, execution_time_ms FROM schema_migrations ORDER BY version ASC`
	rows, err := m.exec.Query(ctx, querySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make(map[int]MigrationRecord)
	for rows.Next() {
		var rec MigrationRecord
		if err := rows.Scan(&rec.Version, &rec.Name, &rec.Checksum, &rec.AppliedAt, &rec.ExecutionTimeMs); err != nil {
			return nil, err
		}
		records[rec.Version] = rec
	}
	return records, rows.Err()
}

func (m *Migrator) registerDefaultMigrations() {
	_ = m.Register([]Migration{
		{
			Version: 1,
			Name:    "create_workflow_definitions",
			Up: `CREATE TABLE IF NOT EXISTS workflow_definitions (
				namespace TEXT NOT NULL DEFAULT 'default',
				name TEXT NOT NULL,
				version INT NOT NULL DEFAULT 1,
				steps JSONB NOT NULL DEFAULT '[]',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (namespace, name, version)
			)`,
			Down: `DROP TABLE IF EXISTS workflow_definitions`,
		},
		{
			Version: 2,
			Name:    "create_runs",
			Up: `CREATE TABLE IF NOT EXISTS runs (
				namespace TEXT NOT NULL DEFAULT 'default',
				id TEXT NOT NULL,
				workflow_name TEXT NOT NULL,
				workflow_version INT NOT NULL DEFAULT 1,
				state TEXT NOT NULL DEFAULT 'PENDING',
				input JSONB DEFAULT '{}',
				steps JSONB DEFAULT '[]',
				error TEXT DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				started_at TIMESTAMPTZ,
				finished_at TIMESTAMPTZ,
				PRIMARY KEY (namespace, id)
			)`,
			Down: `DROP TABLE IF EXISTS runs`,
		},
		{
			Version: 3,
			Name:    "create_queue",
			Up: `CREATE TABLE IF NOT EXISTS queue (
				namespace TEXT NOT NULL DEFAULT 'default',
				run_id TEXT NOT NULL,
				step_id TEXT NOT NULL,
				enqueued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				visible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				attempt INT NOT NULL DEFAULT 0
			)`,
			Down: `DROP TABLE IF EXISTS queue`,
		},
		{
			Version: 4,
			Name:    "create_events",
			Up: `CREATE TABLE IF NOT EXISTS events (
				id TEXT NOT NULL,
				type TEXT NOT NULL,
				version INT NOT NULL DEFAULT 1,
				namespace TEXT NOT NULL DEFAULT 'default',
				run_id TEXT NOT NULL DEFAULT '',
				step_id TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				attempt INT NOT NULL DEFAULT 0,
				payload JSONB DEFAULT '{}',
				PRIMARY KEY (id)
			)`,
			Down: `DROP TABLE IF EXISTS events`,
		},
		{
			Version: 5,
			Name:    "create_api_tokens",
			Up: `CREATE TABLE IF NOT EXISTS api_tokens (
				id TEXT NOT NULL,
				namespace TEXT NOT NULL DEFAULT 'default',
				role TEXT NOT NULL DEFAULT 'reader',
				label TEXT DEFAULT '',
				token_hash TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				expires_at TIMESTAMPTZ,
				PRIMARY KEY (id)
			)`,
			Down: `DROP TABLE IF EXISTS api_tokens`,
		},
		{
			Version: 6,
			Name:    "create_webhooks",
			Up: `CREATE TABLE IF NOT EXISTS webhooks (
				id TEXT NOT NULL,
				namespace TEXT NOT NULL DEFAULT 'default',
				url TEXT NOT NULL,
				secret TEXT DEFAULT '',
				types JSONB DEFAULT '[]',
				active BOOLEAN NOT NULL DEFAULT true,
				timeout BIGINT DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (id)
			)`,
			Down: `DROP TABLE IF EXISTS webhooks`,
		},
		{
			Version: 7,
			Name:    "create_deliveries",
			Up: `CREATE TABLE IF NOT EXISTS deliveries (
				id TEXT NOT NULL,
				event_id TEXT NOT NULL,
				endpoint_id TEXT NOT NULL DEFAULT '',
				namespace TEXT NOT NULL DEFAULT 'default',
				status TEXT NOT NULL DEFAULT 'queued',
				attempts INT NOT NULL DEFAULT 0,
				last_status INT DEFAULT 0,
				last_error TEXT DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				attempted_at TIMESTAMPTZ,
				finished_at TIMESTAMPTZ,
				PRIMARY KEY (id)
			)`,
			Down: `DROP TABLE IF EXISTS deliveries`,
		},
		{
			Version: 8,
			Name:    "create_deadletter_and_artifacts",
			Up: `CREATE TABLE IF NOT EXISTS deadletter (
				id TEXT NOT NULL,
				namespace TEXT NOT NULL DEFAULT 'default',
				run_id TEXT NOT NULL,
				step_id TEXT NOT NULL,
				reason TEXT NOT NULL,
				payload JSONB DEFAULT '{}',
				failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (id)
			);
			CREATE TABLE IF NOT EXISTS step_artifacts (
				id TEXT NOT NULL,
				namespace TEXT NOT NULL DEFAULT 'default',
				run_id TEXT NOT NULL,
				step_id TEXT NOT NULL,
				key TEXT NOT NULL,
				content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
				size_bytes BIGINT NOT NULL DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (id)
			);`,
			Down: `DROP TABLE IF EXISTS deadletter; DROP TABLE IF EXISTS step_artifacts;`,
		},
	}...)
}

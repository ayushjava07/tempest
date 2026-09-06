package migration

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Migrator struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Migrator {
	return &Migrator{pool: pool}
}

func (m *Migrator) Migrate(ctx context.Context) error {
	migrations := []struct {
		name string
		sql  string
	}{
		{"001_create_workflow_definitions", `
			CREATE TABLE IF NOT EXISTS workflow_definitions (
				namespace TEXT NOT NULL DEFAULT 'default',
				name TEXT NOT NULL,
				version INT NOT NULL DEFAULT 1,
				steps JSONB NOT NULL DEFAULT '[]',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (namespace, name, version)
			)`},
		{"002_create_runs", `
			CREATE TABLE IF NOT EXISTS runs (
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
			)`},
		{"003_create_queue", `
			CREATE TABLE IF NOT EXISTS queue (
				namespace TEXT NOT NULL DEFAULT 'default',
				run_id TEXT NOT NULL,
				step_id TEXT NOT NULL,
				enqueued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				visible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				attempt INT NOT NULL DEFAULT 0
			)`},
		{"004_create_events", `
			CREATE TABLE IF NOT EXISTS events (
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
			)`},
		{"005_create_api_tokens", `
			CREATE TABLE IF NOT EXISTS api_tokens (
				id TEXT NOT NULL,
				namespace TEXT NOT NULL DEFAULT 'default',
				role TEXT NOT NULL DEFAULT 'reader',
				label TEXT DEFAULT '',
				token_hash TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				expires_at TIMESTAMPTZ,
				PRIMARY KEY (id)
			)`},
		{"006_create_webhooks", `
			CREATE TABLE IF NOT EXISTS webhooks (
				id TEXT NOT NULL,
				namespace TEXT NOT NULL DEFAULT 'default',
				url TEXT NOT NULL,
				secret TEXT DEFAULT '',
				types JSONB DEFAULT '[]',
				active BOOLEAN NOT NULL DEFAULT true,
				timeout BIGINT DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (id)
			)`},
		{"007_create_deliveries", `
			CREATE TABLE IF NOT EXISTS deliveries (
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
			)`},
		{"008_create_schema_migrations", `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version INT NOT NULL,
				name TEXT NOT NULL,
				applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (version)
			)`},
	}
	for i, mig := range migrations {
		var exists bool
		err := m.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, i+1,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", mig.name, err)
		}
		if exists {
			continue
		}
		if _, err := m.pool.Exec(ctx, mig.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", mig.name, err)
		}
		if _, err := m.pool.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1,$2)`, i+1, mig.name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", mig.name, err)
		}
	}
	return nil
}

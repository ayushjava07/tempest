package migration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// mockExecutor implements Executor in memory for deterministic test verification.
type mockExecutor struct {
	mu          sync.Mutex
	locked      bool
	lockID      int64
	tables      map[string]bool
	records     map[int]MigrationRecord
	execHistory []string
	failExec    bool
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		tables:  make(map[string]bool),
		records: make(map[int]MigrationRecord),
	}
}

func (m *mockExecutor) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failExec {
		return 0, errors.New("simulated exec failure")
	}

	trimmed := strings.TrimSpace(sql)
	m.execHistory = append(m.execHistory, trimmed)

	if strings.HasPrefix(trimmed, "CREATE TABLE IF NOT EXISTS schema_migrations") {
		m.tables["schema_migrations"] = true
		return 0, nil
	}
	if strings.HasPrefix(trimmed, "INSERT INTO schema_migrations") {
		if len(args) >= 5 {
			ver := args[0].(int)
			name := args[1].(string)
			checksum := args[2].(string)
			appliedAt := args[3].(time.Time)
			execTime := args[4].(int64)
			m.records[ver] = MigrationRecord{
				Version:         ver,
				Name:            name,
				Checksum:        checksum,
				AppliedAt:       appliedAt,
				ExecutionTimeMs: execTime,
			}
		}
		return 1, nil
	}
	if strings.HasPrefix(trimmed, "DELETE FROM schema_migrations") {
		if len(args) >= 1 {
			ver := args[0].(int)
			delete(m.records, ver)
		}
		return 1, nil
	}
	return 1, nil
}

func (m *mockExecutor) QueryRow(ctx context.Context, sql string, args ...any) (RowScanner, error) {
	return nil, errors.New("unexpected QueryRow in mock")
}

func (m *mockExecutor) Query(ctx context.Context, sql string, args ...any) (RowsScanner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	trimmed := strings.TrimSpace(sql)
	if strings.HasPrefix(trimmed, "SELECT version, name, checksum, applied_at, execution_time_ms FROM schema_migrations") {
		var list []MigrationRecord
		for _, r := range m.records {
			list = append(list, r)
		}
		// Sort ascending
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if list[i].Version > list[j].Version {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
		return &mockRows{records: list, index: -1}, nil
	}
	return nil, fmt.Errorf("unsupported mock query: %s", sql)
}

func (m *mockExecutor) AcquireLock(ctx context.Context, lockID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked {
		return ErrLockAcquisitionFailed
	}
	m.locked = true
	m.lockID = lockID
	return nil
}

func (m *mockExecutor) ReleaseLock(ctx context.Context, lockID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked && m.lockID == lockID {
		m.locked = false
	}
	return nil
}

type mockRows struct {
	records []MigrationRecord
	index   int
}

func (r *mockRows) Next() bool {
	r.index++
	return r.index < len(r.records)
}

func (r *mockRows) Scan(dest ...any) error {
	if r.index < 0 || r.index >= len(r.records) {
		return errors.New("out of bounds")
	}
	rec := r.records[r.index]
	*(dest[0].(*int)) = rec.Version
	*(dest[1].(*string)) = rec.Name
	*(dest[2].(*string)) = rec.Checksum
	*(dest[3].(*time.Time)) = rec.AppliedAt
	*(dest[4].(*int64)) = rec.ExecutionTimeMs
	return nil
}

func (r *mockRows) Close()       {}
func (r *mockRows) Err() error   { return nil }

func TestMigrator_DefaultMigrations(t *testing.T) {
	mock := newMockExecutor()
	m := NewWithExecutor(mock)

	ctx := context.Background()
	err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if !mock.tables["schema_migrations"] {
		t.Error("expected schema_migrations table to be created")
	}

	if len(mock.records) != 8 {
		t.Fatalf("expected 8 applied migrations, got %d", len(mock.records))
	}

	status, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	for _, st := range status {
		if !st.Applied {
			t.Errorf("migration %d (%s) should be applied", st.Version, st.Name)
		}
		if st.AppliedAt == nil {
			t.Errorf("migration %d (%s) missing AppliedAt timestamp", st.Version, st.Name)
		}
	}

	// Idempotent rerun
	err = m.Migrate(ctx)
	if err != nil {
		t.Fatalf("idempotent Migrate rerun failed: %v", err)
	}
	if len(mock.records) != 8 {
		t.Fatalf("expected still 8 applied migrations, got %d", len(mock.records))
	}
}

func TestMigrator_RegistrationValidation(t *testing.T) {
	mock := newMockExecutor()
	m := NewWithExecutor(mock)

	// Duplicate version
	err := m.Register(Migration{Version: 8, Name: "duplicate", Up: "SELECT 1;"})
	if !errors.Is(err, ErrDuplicateVersion) {
		t.Errorf("expected ErrDuplicateVersion, got %v", err)
	}

	// Non monotonic version
	err = m.Register(Migration{Version: 5, Name: "older", Up: "SELECT 1;"})
	if !errors.Is(err, ErrNonMonotonicVersion) {
		t.Errorf("expected ErrNonMonotonicVersion, got %v", err)
	}

	// Empty name
	err = m.Register(Migration{Version: 9, Name: "", Up: "SELECT 1;"})
	if !errors.Is(err, ErrEmptyMigrationName) {
		t.Errorf("expected ErrEmptyMigrationName, got %v", err)
	}

	// Empty Up
	err = m.Register(Migration{Version: 9, Name: "no_up", Up: ""})
	if !errors.Is(err, ErrEmptyUpSQL) {
		t.Errorf("expected ErrEmptyUpSQL, got %v", err)
	}
}

func TestMigrator_ChecksumMismatch(t *testing.T) {
	mock := newMockExecutor()
	m := NewWithExecutor(mock)

	ctx := context.Background()
	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("initial Migrate failed: %v", err)
	}

	// Tamper with applied checksum in database
	rec := mock.records[1]
	rec.Checksum = "tampered_checksum_value"
	mock.records[1] = rec

	// Next migrate should detect checksum tampering
	err := m.Migrate(ctx)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestMigrator_Rollback(t *testing.T) {
	mock := newMockExecutor()
	m := NewWithExecutor(mock)

	ctx := context.Background()
	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	rolledBack, err := m.Rollback(ctx, 2)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if rolledBack != 2 {
		t.Errorf("expected 2 rolled back migrations, got %d", rolledBack)
	}

	if len(mock.records) != 6 {
		t.Errorf("expected 6 remaining applied migrations, got %d", len(mock.records))
	}

	// Migration 7 and 8 should now be unapplied
	statuses, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	for _, st := range statuses {
		if st.Version > 6 && st.Applied {
			t.Errorf("migration %d should be unapplied after rollback", st.Version)
		}
		if st.Version <= 6 && !st.Applied {
			t.Errorf("migration %d should remain applied", st.Version)
		}
	}
}

func TestMigrator_LockContention(t *testing.T) {
	mock := newMockExecutor()
	m := NewWithExecutor(mock)

	ctx := context.Background()
	// Pre-lock
	_ = mock.AcquireLock(ctx, DefaultAdvisoryLockID)

	err := m.Migrate(ctx)
	if !errors.Is(err, ErrLockAcquisitionFailed) {
		t.Errorf("expected ErrLockAcquisitionFailed when already locked, got %v", err)
	}

	_ = mock.ReleaseLock(ctx, DefaultAdvisoryLockID)
	// Now should succeed
	if err := m.Migrate(ctx); err != nil {
		t.Errorf("expected migration to succeed after lock released, got %v", err)
	}
}

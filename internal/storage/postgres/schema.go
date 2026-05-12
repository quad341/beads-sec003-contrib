package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// bdSchemaMigrationLockID identifies the advisory lock used for first-connect
// schema migrations. Constant 64-bit value derived once from the FNV-1a hash
// of "beads-schema-migration"; never derived at runtime so concurrent migrators
// across bd versions always serialize on the same key.
const bdSchemaMigrationLockID int64 = 0x6265616473736370

//go:embed migrations/0001_initial.up.sql
var initialUpSQL string

//go:embed migrations/0002_drop_child_counters_fk.up.sql
var dropChildCountersFKSQL string

//go:embed migrations/0004_rename_friendly_fks.up.sql
var renameFriendlyFKsSQL string

// embeddedMigration carries a versioned migration step. Either Body (raw SQL)
// or Apply (Go func with a tx) is set; runMigrations prefers Apply when both
// are set, though in practice each migration uses exactly one.
type embeddedMigration struct {
	Version int
	Body    string
	Apply   func(ctx context.Context, tx pgx.Tx) error
}

var embeddedMigrations = []embeddedMigration{
	{Version: 1, Body: initialUpSQL},
	{Version: 2, Body: dropChildCountersFKSQL},
	{Version: 3, Apply: backfillCustomTablesV3},
	{Version: 4, Body: renameFriendlyFKsSQL},
}

// runMigrations brings the database schema up to the latest embedded version.
// In ReadOnly mode it only verifies the schema is current and returns
// ErrSchemaOutOfDate otherwise.
func (s *PostgresStore) runMigrations(ctx context.Context, readOnly bool) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return wrapErr("acquire migration conn", err)
	}
	defer conn.Release()

	if readOnly {
		// Read-only: just confirm schema is at or beyond the latest embedded version.
		var current int
		err := conn.QueryRow(ctx,
			`SELECT COALESCE(MAX(version), 0) FROM bd_schema_migrations`,
		).Scan(&current)
		if err != nil {
			// bd_schema_migrations might not exist on a fresh DB — treat as out of date.
			return ErrSchemaOutOfDate
		}
		if current < latestEmbeddedVersion() {
			return ErrSchemaOutOfDate
		}
		return nil
	}

	// Acquire advisory lock; concurrent processes block here until release.
	if _, err := conn.Exec(ctx,
		"SELECT pg_advisory_lock($1)", bdSchemaMigrationLockID,
	); err != nil {
		return wrapErr("acquire advisory lock", err)
	}
	defer func() {
		// Release on the same connection that acquired it; pool releasing
		// returns the connection to the pool with the session lock still
		// held, so we explicitly release. Use a fresh context with a short
		// timeout — if the caller canceled the parent ctx mid-migration,
		// reusing it would no-op the unlock and leak the lock to the next
		// process that tries to migrate.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", bdSchemaMigrationLockID)
	}()

	// Ensure bookkeeping table exists.
	if _, err := conn.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS bd_schema_migrations (
            version    INTEGER PRIMARY KEY,
            applied_at TIMESTAMP NOT NULL DEFAULT NOW()
        )
    `); err != nil {
		return wrapErr("create bd_schema_migrations", err)
	}

	var current int
	if err := conn.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM bd_schema_migrations`,
	).Scan(&current); err != nil {
		return wrapErr("read bd_schema_migrations", err)
	}

	for _, m := range embeddedMigrations {
		if m.Version <= current {
			continue
		}
		// Each migration runs in its own transaction so a failure halfway
		// through one cannot leave the bookkeeping table out of sync.
		tx, err := conn.Begin(ctx)
		if err != nil {
			return wrapErr(fmt.Sprintf("begin migration %d", m.Version), err)
		}
		if m.Apply != nil {
			if err := m.Apply(ctx, tx); err != nil {
				_ = tx.Rollback(ctx)
				return wrapErr(fmt.Sprintf("apply migration %d", m.Version), err)
			}
		} else if _, err := tx.Exec(ctx, m.Body); err != nil {
			_ = tx.Rollback(ctx)
			return wrapErr(fmt.Sprintf("apply migration %d", m.Version), err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO bd_schema_migrations(version) VALUES ($1)`, m.Version,
		); err != nil {
			_ = tx.Rollback(ctx)
			return wrapErr(fmt.Sprintf("record migration %d", m.Version), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return wrapErr(fmt.Sprintf("commit migration %d", m.Version), err)
		}
	}
	return nil
}

func latestEmbeddedVersion() int {
	if len(embeddedMigrations) == 0 {
		return 0
	}
	return embeddedMigrations[len(embeddedMigrations)-1].Version
}

// backfillCustomTablesV3 populates custom_types and custom_statuses from the
// pg config table when the normalized tables are empty. Mirrors dolt
// migration 016 (BackfillCustomTables) for pg-only databases that were
// initialized before the runtime sync hook landed.
//
// Each helper short-circuits if its target table already has rows so manual
// repair or partial earlier sync is preserved. Re-running the migration is a
// no-op once the bookkeeping row is in place.
//
// Parse errors on status.custom are logged and skipped (matching dolt 016) so
// a corrupt config row cannot block schema migration on a live database.
func backfillCustomTablesV3(ctx context.Context, tx pgx.Tx) error {
	if err := backfillTypesV3(ctx, tx); err != nil {
		return fmt.Errorf("backfill custom_types: %w", err)
	}
	if err := backfillStatusesV3(ctx, tx); err != nil {
		return fmt.Errorf("backfill custom_statuses: %w", err)
	}
	return nil
}

func backfillTypesV3(ctx context.Context, tx pgx.Tx) error {
	var count int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM custom_types").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var value string
	if err := tx.QueryRow(ctx,
		"SELECT value FROM config WHERE key = 'types.custom'",
	).Scan(&value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if value == "" {
		return nil
	}
	for _, name := range issueops.ParseTypesValue(value) {
		if _, err := tx.Exec(ctx,
			"INSERT INTO custom_types (name) VALUES ($1) ON CONFLICT (name) DO NOTHING",
			name,
		); err != nil {
			return fmt.Errorf("insert %q: %w", name, err)
		}
	}
	return nil
}

func backfillStatusesV3(ctx context.Context, tx pgx.Tx) error {
	var count int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM custom_statuses").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var value string
	if err := tx.QueryRow(ctx,
		"SELECT value FROM config WHERE key = 'status.custom'",
	).Scan(&value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if value == "" {
		return nil
	}
	parsed, parseErr := types.ParseCustomStatusConfig(value)
	if parseErr != nil {
		log.Printf("migration 0003: skipping invalid status.custom entries: %v", parseErr)
		return nil
	}
	for _, s := range parsed {
		if _, err := tx.Exec(ctx,
			"INSERT INTO custom_statuses (name, category) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING",
			s.Name, string(s.Category),
		); err != nil {
			return fmt.Errorf("insert %q: %w", s.Name, err)
		}
	}
	return nil
}

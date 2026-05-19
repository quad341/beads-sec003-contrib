//go:build cgo

package schema_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/storage/schema"
)

// openSchemaConn opens a fresh embedded Dolt database for schema testing.
// The database is a brand new directory with no schema applied.
// Returns a *sql.Conn and a cleanup function.
func openSchemaConn(t *testing.T) (*sql.Conn, func()) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "doltdata")
	ctx := context.Background()

	db, cleanup, err := embeddeddolt.OpenSQL(ctx, dataDir, "", "")
	if err != nil {
		t.Fatalf("OpenSQL: %v", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		_ = cleanup()
		t.Fatalf("db.Conn: %v", err)
	}

	return conn, func() {
		conn.Close()
		_ = cleanup()
	}
}

// TestPendingMigrationCount_ReturnsNonZeroWhenPending verifies that a fresh
// (non-migrated) database reports PendingMigrationCount > 0.
func TestPendingMigrationCount_ReturnsNonZeroWhenPending(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt schema tests")
	}
	t.Parallel()

	ctx := context.Background()
	conn, cleanup := openSchemaConn(t)
	defer cleanup()

	// No migrations applied to the fresh db — should report all migrations as pending.
	pending, err := schema.PendingMigrationCount(ctx, conn)
	if err != nil {
		t.Fatalf("PendingMigrationCount on fresh db: %v", err)
	}

	latestVer := schema.LatestVersion()
	if pending <= 0 {
		t.Errorf("expected PendingMigrationCount > 0 on fresh db, got %d (LatestVersion=%d)",
			pending, latestVer)
	}
}

// TestPendingMigrationCount_ReturnsZeroWhenCurrent verifies that after
// applying all migrations, PendingMigrationCount returns 0.
func TestPendingMigrationCount_ReturnsZeroWhenCurrent(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt schema tests")
	}
	t.Parallel()

	ctx := context.Background()
	conn, cleanup := openSchemaConn(t)
	defer cleanup()

	// Apply all migrations.
	if _, err := schema.MigrateUp(ctx, conn); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	// After all migrations: should report 0 pending.
	pending, err := schema.PendingMigrationCount(ctx, conn)
	if err != nil {
		t.Fatalf("PendingMigrationCount after full migration: %v", err)
	}
	if pending != 0 {
		t.Errorf("expected PendingMigrationCount == 0 after full migration, got %d", pending)
	}
}

// TestPendingMigrationCount_PartialMigration verifies that after applying
// only some migrations, PendingMigrationCount reports the remaining count.
func TestPendingMigrationCount_PartialMigration(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt schema tests")
	}
	t.Parallel()

	ctx := context.Background()
	conn, cleanup := openSchemaConn(t)
	defer cleanup()

	// Get the initial pending count (should be LatestVersion()).
	totalPending, err := schema.PendingMigrationCount(ctx, conn)
	if err != nil {
		t.Fatalf("initial PendingMigrationCount: %v", err)
	}
	if totalPending == 0 {
		t.Skip("no migrations to apply — schema tests not meaningful")
	}

	// Apply all migrations.
	applied, err := schema.MigrateUp(ctx, conn)
	if err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	if applied != totalPending {
		t.Errorf("MigrateUp applied %d, want %d (totalPending)", applied, totalPending)
	}

	// Now simulate a partial rollback by deleting the latest migration.
	if _, err := conn.ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)"); err != nil {
		t.Fatalf("DELETE latest migration: %v", err)
	}

	// PendingMigrationCount should now be 1.
	pending, err := schema.PendingMigrationCount(ctx, conn)
	if err != nil {
		t.Fatalf("PendingMigrationCount after partial rollback: %v", err)
	}
	if pending != 1 {
		t.Errorf("expected PendingMigrationCount == 1 after removing latest migration, got %d", pending)
	}
}

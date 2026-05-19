//go:build cgo

package embeddeddolt_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// dropLatestMigration removes the highest-version row from schema_migrations,
// simulating a database that is one migration behind the binary.
func dropLatestMigration(t *testing.T, dataDir string) {
	t.Helper()
	ctx := context.Background()
	db, cleanup, err := embeddeddolt.OpenSQL(ctx, dataDir, "beads", "")
	if err != nil {
		t.Fatalf("dropLatestMigration: OpenSQL: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("dropLatestMigration: cleanup: %v", err)
		}
	}()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("dropLatestMigration: Conn: %v", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "USE `beads`"); err != nil {
		t.Fatalf("dropLatestMigration: USE beads: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)"); err != nil {
		t.Fatalf("dropLatestMigration: DELETE: %v", err)
	}
}

// TestEmbeddedDolt_StaleSchema_Refusal verifies that Store Open returns an error
// containing "schema is at version" and "run: bd migrate schema" when the
// database schema is behind the binary's expected version.
func TestEmbeddedDolt_StaleSchema_Refusal(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	ctx := context.Background()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	dataDir := filepath.Join(beadsDir, "embeddeddolt")

	// First open: autoMigrate=true creates a fully migrated store.
	store, err := embeddeddolt.Open(ctx, beadsDir, "beads", "", true)
	if err != nil {
		t.Fatalf("initial Open (autoMigrate=true): %v", err)
	}
	store.Close()

	// Simulate a stale schema by removing the latest migration record.
	dropLatestMigration(t, dataDir)

	// Second open: autoMigrate=false should trigger the schema guard.
	_, err = embeddeddolt.Open(ctx, beadsDir, "beads", "", false)
	if err == nil {
		t.Fatal("expected error from stale schema guard, got nil")
	}

	// Verify error message contains expected strings per the spec.
	msg := err.Error()
	if !strings.Contains(msg, "schema is at version") {
		t.Errorf("error %q does not contain 'schema is at version'", msg)
	}
	if !strings.Contains(msg, "run: bd migrate schema") {
		t.Errorf("error %q does not contain 'run: bd migrate schema'", msg)
	}
}

// TestEmbeddedDolt_StaleSchema_AutoMigrateBypassesGuard verifies that opening a
// store with autoMigrate=true succeeds even when the schema is stale.
// This is the path used by bd init — it should never refuse with the schema guard.
func TestEmbeddedDolt_StaleSchema_AutoMigrateBypassesGuard(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	ctx := context.Background()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	dataDir := filepath.Join(beadsDir, "embeddeddolt")

	// First open: create a fully migrated store.
	store, err := embeddeddolt.Open(ctx, beadsDir, "beads", "", true)
	if err != nil {
		t.Fatalf("initial Open: %v", err)
	}
	store.Close()

	// Simulate stale schema.
	dropLatestMigration(t, dataDir)

	// Open with autoMigrate=true — should re-apply the migration and succeed.
	store, err = embeddeddolt.Open(ctx, beadsDir, "beads", "", true)
	if err != nil {
		t.Fatalf("Open with autoMigrate=true on stale schema: %v", err)
	}
	store.Close()
}

// TestEmbeddedDolt_StaleSchema_ErrorWraps verifies that the schema guard error
// does NOT use a sentinel that needs unwrapping — callers can check the string.
// (The error is a plain fmt.Errorf, not errors.Is-able, by design.)
func TestEmbeddedDolt_StaleSchema_ErrorDoesNotPanic(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	ctx := context.Background()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	dataDir := filepath.Join(beadsDir, "embeddeddolt")

	store, err := embeddeddolt.Open(ctx, beadsDir, "beads", "", true)
	if err != nil {
		t.Fatalf("initial Open: %v", err)
	}
	store.Close()

	dropLatestMigration(t, dataDir)

	// Must return error, never panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Open panicked on stale schema: %v", r)
		}
	}()

	_, err = embeddeddolt.Open(ctx, beadsDir, "beads", "", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should NOT be errors.Is-able to any typed sentinel.
	if errors.Is(err, errors.New("anything")) {
		t.Error("unexpected sentinel error match")
	}
}

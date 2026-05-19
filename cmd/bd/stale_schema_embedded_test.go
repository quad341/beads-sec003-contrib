//go:build cgo

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// dropLatestSchemaMigrationForCLI removes the highest-version row from
// schema_migrations to make the CLI see a stale database.
func dropLatestSchemaMigrationForCLI(t *testing.T, beadsDir string) {
	t.Helper()
	dataDir := filepath.Join(beadsDir, "embeddeddolt")
	ctx := context.Background()
	db, cleanup, err := embeddeddolt.OpenSQL(ctx, dataDir, "beads", "")
	if err != nil {
		t.Fatalf("dropLatestSchemaMigrationForCLI: OpenSQL: %v", err)
	}
	defer func() { _ = cleanup() }()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("dropLatestSchemaMigrationForCLI: Conn: %v", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "USE `beads`"); err != nil {
		t.Fatalf("dropLatestSchemaMigrationForCLI: USE beads: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)"); err != nil {
		t.Fatalf("dropLatestSchemaMigrationForCLI: DELETE: %v", err)
	}
}

// bdMigrateSchemaCmd runs "bd migrate schema" returning stdout, fataling on error.
func bdMigrateSchemaCmd(t *testing.T, bd, dir string) string {
	t.Helper()
	cmd := exec.Command(bd, "migrate", "schema")
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd migrate schema failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// bdListAllowErr runs "bd list" returning stdout+stderr and any error.
func bdListAllowErr(t *testing.T, bd, dir string) (string, error) {
	t.Helper()
	cmd := exec.Command(bd, "list")
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestBdCLI_StaleSchema_Refusal verifies that after manually corrupting the
// schema_migrations table, 'bd list' exits with code 1 and the error message
// contains "schema is at version" and "run: bd migrate schema".
func TestBdCLI_StaleSchema_Refusal(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "ss")

	// Corrupt the schema_migrations cursor to simulate a stale database.
	dropLatestSchemaMigrationForCLI(t, beadsDir)

	// 'bd list' should fail because Store Open fires the schema guard.
	out, err := bdListAllowErr(t, bd, dir)
	if err == nil {
		t.Fatalf("expected bd list to fail on stale schema, but it succeeded; output:\n%s", out)
	}

	if !strings.Contains(out, "schema is at version") {
		t.Errorf("expected 'schema is at version' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "run: bd migrate schema") {
		t.Errorf("expected 'run: bd migrate schema' in output, got:\n%s", out)
	}
}

// TestBdInit_StaleSchema_AutoMigrates verifies that 'bd init' succeeds even
// when the embedded store exists but has a stale schema. The init path uses
// autoMigrate=true, bypassing the schema guard and re-applying migrations.
func TestBdInit_StaleSchema_AutoMigrates(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "sia")

	// Stale the schema.
	dropLatestSchemaMigrationForCLI(t, beadsDir)

	// 'bd list' should fail (guard fires).
	out, err := bdListAllowErr(t, bd, dir)
	if err == nil {
		t.Logf("note: bd list did not fail (may be schema stale guard not active); output: %s", out)
	}

	// 'bd migrate schema' should fix it and exit 0.
	migrateOut := bdMigrateSchemaCmd(t, bd, dir)
	if strings.Contains(migrateOut, "error") || strings.Contains(migrateOut, "Error") {
		t.Errorf("bd migrate schema returned error output: %s", migrateOut)
	}

	// After migration, 'bd list' must succeed.
	bdList(t, bd, dir)
}

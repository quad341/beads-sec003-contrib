//go:build cgo

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// bdMigrateSchema runs "bd migrate schema" with the given args and returns stdout.
// Fatals the test on non-zero exit.
func bdMigrateSchema(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"migrate", "schema"}, args...)
	cmd := exec.Command(bd, fullArgs...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd migrate schema %s failed: %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// bdMigrateSchemaAllowErr runs "bd migrate schema" returning stdout even on error.
func bdMigrateSchemaAllowErr(t *testing.T, bd, dir string, args ...string) (string, error) {
	t.Helper()
	fullArgs := append([]string{"migrate", "schema"}, args...)
	cmd := exec.Command(bd, fullArgs...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, _, err := runCommandBuffers(t, cmd)
	return stdout.String(), err
}

// dropLatestSchemaMigration deletes the highest-version row from schema_migrations
// in the embedded dolt database, simulating a database that is one migration behind.
func dropLatestSchemaMigration(t *testing.T, beadsDir string) {
	t.Helper()
	dataDir := filepath.Join(beadsDir, "embeddeddolt")
	ctx := context.Background()
	db, cleanup, err := embeddeddolt.OpenSQL(ctx, dataDir, "beads", "")
	if err != nil {
		t.Fatalf("dropLatestSchemaMigration: OpenSQL: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("dropLatestSchemaMigration: cleanup: %v", err)
		}
	}()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("dropLatestSchemaMigration: Conn: %v", err)
	}
	defer conn.Close()

	// Use beads database
	if _, err := conn.ExecContext(ctx, "USE `beads`"); err != nil {
		t.Fatalf("dropLatestSchemaMigration: USE beads: %v", err)
	}

	// Delete the latest migration to make the schema appear stale
	if _, err := conn.ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)"); err != nil {
		t.Fatalf("dropLatestSchemaMigration: DELETE: %v", err)
	}
}

// TestMigrateSchema_applies verifies that 'bd migrate schema' applies pending
// migrations and that a subsequent run is idempotent ("already up to date").
func TestMigrateSchema_applies(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "msa")

	// Simulate a stale schema by removing the latest migration record.
	dropLatestSchemaMigration(t, beadsDir)

	// First run: should apply the pending migration.
	out := bdMigrateSchema(t, bd, dir)
	if !strings.Contains(out, "migration") {
		t.Errorf("expected migration output after stale schema, got: %q", out)
	}
	if strings.Contains(out, "already up to date") {
		t.Errorf("expected migration to be applied, but got 'already up to date': %q", out)
	}

	// Second run: idempotent — schema is now current.
	out2 := bdMigrateSchema(t, bd, dir)
	if !strings.Contains(out2, "already up to date") {
		t.Errorf("expected 'already up to date' on second run, got: %q", out2)
	}
}

// TestMigrateSchema_jsonOutput verifies that 'bd migrate schema --json' emits
// valid JSON with the required fields: applied (int) and current_version (int).
func TestMigrateSchema_jsonOutput(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "msj")

	out := bdMigrateSchema(t, bd, dir, "--json")
	out = strings.TrimSpace(out)

	// Find JSON object in output
	start := strings.Index(out, "{")
	if start < 0 {
		t.Fatalf("no JSON object in bd migrate schema --json output: %q", out)
	}
	jsonStr := out[start:]

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("bd migrate schema --json: invalid JSON: %v\noutput: %q", err, out)
	}

	// Verify required fields.
	if _, ok := result["applied"]; !ok {
		t.Errorf("bd migrate schema --json missing 'applied' field; got: %v", result)
	}
	if _, ok := result["current_version"]; !ok {
		t.Errorf("bd migrate schema --json missing 'current_version' field; got: %v", result)
	}

	// applied should be a number.
	if applied, ok := result["applied"].(float64); ok {
		if applied < 0 {
			t.Errorf("'applied' must be non-negative, got: %v", applied)
		}
	} else {
		t.Errorf("'applied' must be a number, got type %T: %v", result["applied"], result["applied"])
	}

	// current_version should be a positive number.
	if cv, ok := result["current_version"].(float64); ok {
		if cv <= 0 {
			t.Errorf("'current_version' must be positive, got: %v", cv)
		}
	} else {
		t.Errorf("'current_version' must be a number, got type %T: %v", result["current_version"], result["current_version"])
	}
}

// TestMigrateSchema_thenCommandSucceeds verifies that after running
// 'bd migrate schema', normal commands like 'bd list' continue to work.
func TestMigrateSchema_thenCommandSucceeds(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "msc")

	// Run schema migration.
	bdMigrateSchema(t, bd, dir)

	// Verify bd list succeeds after migration.
	bdList(t, bd, dir)
}

// TestBdInit_stillAutoMigrates verifies that 'bd init' automatically applies
// schema migrations without requiring an explicit 'bd migrate schema'.
// This is the regression guard for the bd init exception (no schema guard bypass needed).
func TestBdInit_stillAutoMigrates(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "msi")

	// After bd init, commands should work without any explicit migration step.
	bdList(t, bd, dir)

	// Also verify migrate schema reports no pending migrations.
	out := bdMigrateSchema(t, bd, dir, "--json")
	out = strings.TrimSpace(out)

	start := strings.Index(out, "{")
	if start >= 0 {
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(out[start:]), &result); err == nil {
			if applied, ok := result["applied"].(float64); ok && applied != 0 {
				t.Errorf("TestBdInit_stillAutoMigrates: expected 0 pending migrations after bd init, got %v", applied)
			}
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// be-ec5 / be-c0s follow-up: pin resolveDoltSourceDataDir's branches in
// migrate.go. The function is pedestrian decision logic with deterministic
// branches and no Dolt/PG runtime dependency — pure file-system probing —
// but it currently has 0% coverage (the be-c0s reviewer relied on
// end-to-end manual verification on mcd-pg-demo).

// makeMetadataJSON writes a minimal metadata.json into beadsDir naming the
// supplied dolt database. Caller must have created beadsDir.
func makeMetadataJSON(t *testing.T, beadsDir, doltDatabase string) {
	t.Helper()
	body := `{"version":"1","backend":"dolt","dolt_database":"` + doltDatabase + `"}`
	path := filepath.Join(beadsDir, "metadata.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}
}

// makeDoltLayout creates <beadsDir>/<layoutDir>/<db>/.dolt so the resolver
// considers it a viable candidate. The actual contents are irrelevant —
// the resolver only checks that .dolt is a directory.
func makeDoltLayout(t *testing.T, beadsDir, layoutDir, db string) {
	t.Helper()
	dotDolt := filepath.Join(beadsDir, layoutDir, db, ".dolt")
	if err := os.MkdirAll(dotDolt, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dotDolt, err)
	}
}

// TestResolveDoltSourceDataDir_ServerLayout: metadata.json + dolt/<db>/.dolt
// → returns the server-mode candidate.
func TestResolveDoltSourceDataDir_ServerLayout(t *testing.T) {
	beadsDir := t.TempDir()
	makeMetadataJSON(t, beadsDir, "beads_test")
	makeDoltLayout(t, beadsDir, "dolt", "beads_test")

	dataDir, db, err := resolveDoltSourceDataDir(beadsDir)
	if err != nil {
		t.Fatalf("resolveDoltSourceDataDir: %v", err)
	}
	if db != "beads_test" {
		t.Errorf("database = %q, want %q", db, "beads_test")
	}
	want := filepath.Join(beadsDir, "dolt")
	abs, _ := filepath.Abs(want)
	if dataDir != abs {
		t.Errorf("dataDir = %q, want %q", dataDir, abs)
	}
}

// TestResolveDoltSourceDataDir_EmbeddedLayout: metadata.json +
// embeddeddolt/<db>/.dolt (no dolt/ tree) → returns the embedded candidate.
func TestResolveDoltSourceDataDir_EmbeddedLayout(t *testing.T) {
	beadsDir := t.TempDir()
	makeMetadataJSON(t, beadsDir, "beads_embed")
	makeDoltLayout(t, beadsDir, "embeddeddolt", "beads_embed")

	dataDir, db, err := resolveDoltSourceDataDir(beadsDir)
	if err != nil {
		t.Fatalf("resolveDoltSourceDataDir: %v", err)
	}
	if db != "beads_embed" {
		t.Errorf("database = %q, want %q", db, "beads_embed")
	}
	want := filepath.Join(beadsDir, "embeddeddolt")
	abs, _ := filepath.Abs(want)
	if dataDir != abs {
		t.Errorf("dataDir = %q, want %q", dataDir, abs)
	}
}

// TestResolveDoltSourceDataDir_BothLayoutsServerWins: when both dolt/ and
// embeddeddolt/ exist, server-mode is preferred (matches the function's
// candidate order).
func TestResolveDoltSourceDataDir_BothLayoutsServerWins(t *testing.T) {
	beadsDir := t.TempDir()
	makeMetadataJSON(t, beadsDir, "beads_both")
	makeDoltLayout(t, beadsDir, "dolt", "beads_both")
	makeDoltLayout(t, beadsDir, "embeddeddolt", "beads_both")

	dataDir, db, err := resolveDoltSourceDataDir(beadsDir)
	if err != nil {
		t.Fatalf("resolveDoltSourceDataDir: %v", err)
	}
	if db != "beads_both" {
		t.Errorf("database = %q, want %q", db, "beads_both")
	}
	want, _ := filepath.Abs(filepath.Join(beadsDir, "dolt"))
	if dataDir != want {
		t.Errorf("expected server-mode candidate to win; dataDir = %q, want %q", dataDir, want)
	}
}

// TestResolveDoltSourceDataDir_NoLayoutErrors: metadata.json present but
// neither dolt/ nor embeddeddolt/ tree → clear error naming the missing
// database.
func TestResolveDoltSourceDataDir_NoLayoutErrors(t *testing.T) {
	beadsDir := t.TempDir()
	makeMetadataJSON(t, beadsDir, "ghost_db")

	dataDir, db, err := resolveDoltSourceDataDir(beadsDir)
	if err == nil {
		t.Fatalf("expected error for source with no dolt layout, got dataDir=%q db=%q",
			dataDir, db)
	}
	msg := err.Error()
	if !strings.Contains(msg, "no dolt database") {
		t.Errorf("expected 'no dolt database' in error, got: %v", err)
	}
	if !strings.Contains(msg, "ghost_db") {
		t.Errorf("expected database name 'ghost_db' in error, got: %v", err)
	}
}

// TestResolveDoltSourceDataDir_NoMetadataDefaultsToBeadsDB: when metadata.json
// is absent (Load returns nil, nil), the resolver falls back to the
// configfile.DefaultDoltDatabase (currently "beads"). This exercises the
// `cfg == nil` branch that the be-c0s reviewer specifically asked for.
func TestResolveDoltSourceDataDir_NoMetadataDefaultsToBeadsDB(t *testing.T) {
	beadsDir := t.TempDir()
	// No metadata.json. Default database is "beads"; lay out dolt/beads/.dolt
	// so resolution can succeed.
	makeDoltLayout(t, beadsDir, "dolt", "beads")

	dataDir, db, err := resolveDoltSourceDataDir(beadsDir)
	if err != nil {
		t.Fatalf("resolveDoltSourceDataDir: %v", err)
	}
	if db != "beads" {
		t.Errorf("expected default database 'beads', got %q", db)
	}
	want, _ := filepath.Abs(filepath.Join(beadsDir, "dolt"))
	if dataDir != want {
		t.Errorf("dataDir = %q, want %q", dataDir, want)
	}
}

// TestResolveDoltSourceDataDir_ConfigLoadErrorPropagates: a corrupt
// metadata.json → wrapped error from configfile.Load surfaces as
// "read --source metadata.json: ...".
func TestResolveDoltSourceDataDir_ConfigLoadErrorPropagates(t *testing.T) {
	beadsDir := t.TempDir()
	bogus := filepath.Join(beadsDir, "metadata.json")
	if err := os.WriteFile(bogus, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write bogus metadata.json: %v", err)
	}

	_, _, err := resolveDoltSourceDataDir(beadsDir)
	if err == nil {
		t.Fatalf("expected error from corrupt metadata.json, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "read --source metadata.json") {
		t.Errorf("expected wrapped error 'read --source metadata.json', got: %v", err)
	}
}

// TestResolveDoltSourceDataDir_IgnoresServerDatabaseEnv: setting
// BEADS_DOLT_SERVER_DATABASE must NOT redirect the resolver. The env var is
// a runtime-mode override consumed by internal/storage/dolt/store.go; the
// migration source is locator-pinned by its own metadata.json. If this
// invariant ever flips, a migration could silently read from the wrong
// database. (be-ec5 explicitly asks to confirm-or-guard this behavior.)
func TestResolveDoltSourceDataDir_IgnoresServerDatabaseEnv(t *testing.T) {
	beadsDir := t.TempDir()
	makeMetadataJSON(t, beadsDir, "from_metadata")
	makeDoltLayout(t, beadsDir, "dolt", "from_metadata")

	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "from_env")

	_, db, err := resolveDoltSourceDataDir(beadsDir)
	if err != nil {
		t.Fatalf("resolveDoltSourceDataDir: %v", err)
	}
	if db != "from_metadata" {
		t.Errorf("env var hijacked the source database: got %q, want %q",
			db, "from_metadata")
	}
}

package configfile_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

// postgresConfig returns a minimal Config with backend=postgres.
func postgresConfig() *configfile.Config {
	return &configfile.Config{
		Backend: configfile.BackendPostgres,
	}
}

// TestDetectLegacyDoltState_NilConfig verifies nil cfg → no output.
func TestDetectLegacyDoltState_NilConfig(t *testing.T) {
	beadsDir := t.TempDir()
	legacyDir, legacyFields := configfile.DetectLegacyDoltState(beadsDir, nil)
	if legacyDir != "" {
		t.Errorf("nil cfg: expected empty legacyDir, got %q", legacyDir)
	}
	if len(legacyFields) != 0 {
		t.Errorf("nil cfg: expected empty legacyFields, got %v", legacyFields)
	}
}

// TestDetectLegacyDoltState_NonPostgresBackend verifies non-postgres backend → no output.
func TestDetectLegacyDoltState_NonPostgresBackend(t *testing.T) {
	beadsDir := t.TempDir()
	// DoltDatabase set — but since backend is dolt (default), should be ignored.
	cfg := &configfile.Config{
		DoltDatabase: "beads",
		DoltMode:     "server",
	}
	legacyDir, legacyFields := configfile.DetectLegacyDoltState(beadsDir, cfg)
	if legacyDir != "" {
		t.Errorf("dolt backend: expected empty legacyDir, got %q", legacyDir)
	}
	if len(legacyFields) != 0 {
		t.Errorf("dolt backend: expected empty legacyFields, got %v", legacyFields)
	}
}

// TestDetectLegacyDoltState_PostgresNoArtifacts verifies postgres backend with
// no dolt dir and no dolt fields → no output.
func TestDetectLegacyDoltState_PostgresNoArtifacts(t *testing.T) {
	beadsDir := t.TempDir()
	cfg := postgresConfig()
	legacyDir, legacyFields := configfile.DetectLegacyDoltState(beadsDir, cfg)
	if legacyDir != "" {
		t.Errorf("clean postgres: expected empty legacyDir, got %q", legacyDir)
	}
	if len(legacyFields) != 0 {
		t.Errorf("clean postgres: expected empty legacyFields, got %v", legacyFields)
	}
}

// TestDetectLegacyDoltState_DoltDirExists verifies that a .beads/dolt directory
// is reported as a legacy dir on a postgres workspace.
func TestDetectLegacyDoltState_DoltDirExists(t *testing.T) {
	beadsDir := t.TempDir()
	doltDir := filepath.Join(beadsDir, "dolt")
	if err := os.Mkdir(doltDir, 0o755); err != nil {
		t.Fatalf("mkdir dolt: %v", err)
	}

	cfg := postgresConfig()
	legacyDir, legacyFields := configfile.DetectLegacyDoltState(beadsDir, cfg)
	if legacyDir != doltDir {
		t.Errorf("legacyDir: got %q, want %q", legacyDir, doltDir)
	}
	if len(legacyFields) != 0 {
		t.Errorf("expected no legacy fields (only dolt dir), got %v", legacyFields)
	}
}

// TestDetectLegacyDoltState_DoltFieldsPopulated verifies all 7 dolt fields
// are reported in legacyFields when populated on a postgres workspace.
func TestDetectLegacyDoltState_DoltFieldsPopulated(t *testing.T) {
	beadsDir := t.TempDir()
	cfg := &configfile.Config{
		Backend:            configfile.BackendPostgres,
		DoltMode:           "server",
		DoltServerHost:     "127.0.0.1",
		DoltServerPort:     3307,
		DoltServerUser:     "root",
		DoltDatabase:       "beads",
		DoltDataDir:        "/some/path",
		GlobalDoltDatabase: "beads_global",
	}

	_, legacyFields := configfile.DetectLegacyDoltState(beadsDir, cfg)

	want := []string{"dolt_mode", "dolt_server_host", "dolt_server_port", "dolt_server_user",
		"dolt_database", "dolt_data_dir", "global_dolt_database"}
	if !reflect.DeepEqual(legacyFields, want) {
		t.Errorf("legacyFields:\n got  %v\n want %v", legacyFields, want)
	}
}

// TestDetectLegacyDoltState_BothDirAndFields verifies that when both a dolt dir
// exists and dolt fields are set, both are reported.
func TestDetectLegacyDoltState_BothDirAndFields(t *testing.T) {
	beadsDir := t.TempDir()
	doltDir := filepath.Join(beadsDir, "dolt")
	if err := os.Mkdir(doltDir, 0o755); err != nil {
		t.Fatalf("mkdir dolt: %v", err)
	}

	cfg := &configfile.Config{
		Backend:        configfile.BackendPostgres,
		DoltMode:       "embedded",
		DoltServerHost: "localhost",
	}

	legacyDir, legacyFields := configfile.DetectLegacyDoltState(beadsDir, cfg)
	if legacyDir != doltDir {
		t.Errorf("legacyDir: got %q, want %q", legacyDir, doltDir)
	}
	if len(legacyFields) == 0 {
		t.Error("expected non-empty legacyFields when dolt fields set on postgres workspace")
	}
}

// TestDetectLegacyDoltState_FileNotDir verifies that a plain file named "dolt"
// does not trigger the legacyDir detection (must be a directory).
func TestDetectLegacyDoltState_FileNotDir(t *testing.T) {
	beadsDir := t.TempDir()
	doltPath := filepath.Join(beadsDir, "dolt")
	// Create a file (not a directory) named "dolt".
	if err := os.WriteFile(doltPath, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := postgresConfig()
	legacyDir, _ := configfile.DetectLegacyDoltState(beadsDir, cfg)
	if legacyDir != "" {
		t.Errorf("file named 'dolt' should not trigger legacyDir detection, got %q", legacyDir)
	}
}

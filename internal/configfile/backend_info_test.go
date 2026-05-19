package configfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

// writeMetadata writes a metadata.json file into beadsDir.
func writeMetadata(t *testing.T, beadsDir string, content string) {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile metadata.json: %v", err)
	}
}

// TestResolveBackendInfo_NoMetadata verifies that a missing metadata.json
// returns the "unconfigured" backend.
func TestResolveBackendInfo_NoMetadata(t *testing.T) {
	beadsDir := t.TempDir()
	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.Backend != "unconfigured" {
		t.Errorf("missing metadata: Backend = %q, want %q", bi.Backend, "unconfigured")
	}
}

// TestResolveBackendInfo_DoltEmbedded verifies dolt embedded mode fields.
func TestResolveBackendInfo_DoltEmbedded(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{"backend":"dolt","dolt_database":"beads"}`)
	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.Backend != "dolt" {
		t.Errorf("Backend = %q, want %q", bi.Backend, "dolt")
	}
	if bi.Mode != "embedded" {
		t.Errorf("Mode = %q, want %q", bi.Mode, "embedded")
	}
	if bi.Database != "beads" {
		t.Errorf("Database = %q, want %q", bi.Database, "beads")
	}
}

// TestResolveBackendInfo_DoltServer verifies dolt server mode fields.
func TestResolveBackendInfo_DoltServer(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{
		"backend":"dolt",
		"dolt_mode":"server",
		"dolt_server_host":"127.0.0.1",
		"dolt_server_port":3307,
		"dolt_server_user":"root",
		"dolt_database":"beads"
	}`)
	// Unset port override so the metadata.json port is used.
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")
	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.Backend != "dolt" {
		t.Errorf("Backend = %q, want %q", bi.Backend, "dolt")
	}
	if bi.Mode != "server" {
		t.Errorf("Mode = %q, want %q", bi.Mode, "server")
	}
	if bi.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", bi.Host, "127.0.0.1")
	}
	// Port is read from metadata.json when no env override.
	if bi.Port <= 0 {
		t.Errorf("Port = %d, want > 0", bi.Port)
	}
}

// TestResolveBackendInfo_Postgres verifies postgres backend fields.
func TestResolveBackendInfo_Postgres(t *testing.T) {
	beadsDir := t.TempDir()
	// Stripped DSN (no password — password comes from env var at runtime).
	writeMetadata(t, beadsDir, `{
		"backend":"postgres",
		"postgres_dsn":"postgres://testuser@127.0.0.1:5432/testdb?sslmode=disable"
	}`)
	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.Backend != "postgres" {
		t.Errorf("Backend = %q, want %q", bi.Backend, "postgres")
	}
	if bi.Mode != "postgres" {
		t.Errorf("Mode = %q, want %q", bi.Mode, "postgres")
	}
	if bi.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", bi.Host, "127.0.0.1")
	}
	if bi.Port != 5432 {
		t.Errorf("Port = %d, want %d", bi.Port, 5432)
	}
	if bi.User != "testuser" {
		t.Errorf("User = %q, want %q", bi.User, "testuser")
	}
	if bi.Database != "testdb" {
		t.Errorf("Database = %q, want %q", bi.Database, "testdb")
	}
}

// TestResolveBackendInfo_PostgresWithEnvOverrides verifies that BEADS_POSTGRES_*
// env vars override DSN fields. BEADS_POSTGRES_PASSWORD is NOT reflected in
// BackendInfo (it's applied at connection time only).
func TestResolveBackendInfo_PostgresWithEnvOverrides(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{
		"backend":"postgres",
		"postgres_dsn":"postgres://user@127.0.0.1:5432/originaldb?sslmode=disable"
	}`)

	t.Setenv("BEADS_POSTGRES_HOST", "staging.example.com")
	t.Setenv("BEADS_POSTGRES_PORT", "5433")
	t.Setenv("BEADS_POSTGRES_DATABASE", "stagingdb")

	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.Host != "staging.example.com" {
		t.Errorf("Host (env override): got %q, want %q", bi.Host, "staging.example.com")
	}
	if bi.Port != 5433 {
		t.Errorf("Port (env override): got %d, want %d", bi.Port, 5433)
	}
	if bi.Database != "stagingdb" {
		t.Errorf("Database (env override): got %q, want %q", bi.Database, "stagingdb")
	}
}

// TestResolveBackendInfo_PostgresPasswordNotInBackendInfo verifies that
// BEADS_POSTGRES_PASSWORD is NOT included in BackendInfo fields (security check).
func TestResolveBackendInfo_PostgresPasswordNotInBackendInfo(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{
		"backend":"postgres",
		"postgres_dsn":"postgres://user@127.0.0.1:5432/db?sslmode=disable"
	}`)

	secret := "super-secret-password-abc123"
	t.Setenv("BEADS_POSTGRES_PASSWORD", secret)

	bi := configfile.ResolveBackendInfo(beadsDir)

	// Password must NOT appear in any BackendInfo field.
	for _, val := range []string{bi.Host, bi.User, bi.Database, bi.SSLMode, bi.Mode, bi.Backend} {
		if val == secret {
			t.Errorf("BackendInfo field contains BEADS_POSTGRES_PASSWORD: %q", val)
		}
	}
}

// TestResolveBackendInfo_LegacyDoltFieldsOnPostgres verifies that LegacyDoltDir
// and LegacyDoltFields are populated when postgres workspace has dolt artifacts.
func TestResolveBackendInfo_LegacyDoltFieldsOnPostgres(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{
		"backend":"postgres",
		"postgres_dsn":"postgres://user@127.0.0.1:5432/db?sslmode=disable",
		"dolt_mode":"server",
		"dolt_database":"beads"
	}`)
	// Create the stale dolt directory.
	if err := os.Mkdir(filepath.Join(beadsDir, "dolt"), 0o755); err != nil {
		t.Fatalf("mkdir dolt: %v", err)
	}

	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.LegacyDoltDir == "" {
		t.Error("expected non-empty LegacyDoltDir for postgres workspace with .beads/dolt")
	}
	if len(bi.LegacyDoltFields) == 0 {
		t.Error("expected non-empty LegacyDoltFields for postgres workspace with dolt_mode/dolt_database")
	}
}

// TestResolveBackendInfo_MalformedMetadata verifies that a malformed metadata.json
// returns the "unknown" backend with a ParseError.
func TestResolveBackendInfo_MalformedMetadata(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{not valid json`)
	bi := configfile.ResolveBackendInfo(beadsDir)
	if bi.Backend != "unknown" {
		t.Errorf("malformed JSON: Backend = %q, want %q", bi.Backend, "unknown")
	}
	if bi.ParseError == "" {
		t.Error("malformed JSON: expected ParseError to be set")
	}
}

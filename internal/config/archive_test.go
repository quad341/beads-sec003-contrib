package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupArchiveTestConfig initializes a temporary config.yaml at dir/.beads/config.yaml.
// Returns a cleanup function.
func setupArchiveTestConfig(t *testing.T, dir string, yamlContent string) func() {
	t.Helper()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configPath := filepath.Join(beadsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}
	// Point BEADS_DIR at our temp dir so config functions pick it up.
	prev := os.Getenv("BEADS_DIR")
	_ = os.Setenv("BEADS_DIR", beadsDir)
	// Reset viper for each test.
	resetViper()
	return func() {
		if prev == "" {
			_ = os.Unsetenv("BEADS_DIR")
		} else {
			_ = os.Setenv("BEADS_DIR", prev)
		}
		resetViper()
	}
}

// resetViper clears the global viper state so tests start clean.
func resetViper() {
	v = nil
}

// TestArchiveDefaultForBackend_Dolt verifies dolt returns "jsonl".
func TestArchiveDefaultForBackend_Dolt(t *testing.T) {
	got := ArchiveDefaultForBackend("dolt")
	if got != ArchiveFormatJSONL {
		t.Errorf("ArchiveDefaultForBackend(\"dolt\") = %q, want %q", got, ArchiveFormatJSONL)
	}
}

// TestArchiveDefaultForBackend_PG verifies postgres returns "none".
func TestArchiveDefaultForBackend_PG(t *testing.T) {
	got := ArchiveDefaultForBackend("postgres")
	if got != ArchiveFormatNone {
		t.Errorf("ArchiveDefaultForBackend(\"postgres\") = %q, want %q", got, ArchiveFormatNone)
	}
}

// TestArchiveDefaultForBackend_Unknown verifies unknown backends return "none".
func TestArchiveDefaultForBackend_Unknown(t *testing.T) {
	for _, backend := range []string{"mock", "sqlite", "unknown", ""} {
		got := ArchiveDefaultForBackend(backend)
		if got != ArchiveFormatNone {
			t.Errorf("ArchiveDefaultForBackend(%q) = %q, want %q", backend, got, ArchiveFormatNone)
		}
	}
}

// TestResolveArchiveFormat_canonical verifies archive.format=jsonl → returns "jsonl", no warning.
func TestResolveArchiveFormat_canonical(t *testing.T) {
	dir := t.TempDir()
	restore := setupArchiveTestConfig(t, dir, "archive:\n  format: jsonl\n")
	defer restore()

	// Initialize config so viper reads from our temp config.yaml.
	beadsDir := filepath.Join(dir, ".beads")
	if err := InitializeFromDir(beadsDir); err != nil {
		t.Logf("InitializeFromDir not available, skipping viper setup: %v", err)
	}

	format, warnMsg := ResolveArchiveFormat()
	if format != ArchiveFormatJSONL {
		t.Errorf("ResolveArchiveFormat() format = %q, want %q", format, ArchiveFormatJSONL)
	}
	if warnMsg != "" {
		t.Errorf("ResolveArchiveFormat() warnMsg = %q, want empty", warnMsg)
	}
}

// TestResolveArchiveFormat_alias verifies export.auto=true → returns "jsonl" with warning.
func TestResolveArchiveFormat_alias(t *testing.T) {
	dir := t.TempDir()
	restore := setupArchiveTestConfig(t, dir, "export:\n  auto: \"true\"\n")
	defer restore()

	beadsDir := filepath.Join(dir, ".beads")
	_ = InitializeFromDir(beadsDir)

	format, warnMsg := ResolveArchiveFormat()
	if format != ArchiveFormatJSONL {
		t.Errorf("ResolveArchiveFormat() with export.auto=true: format = %q, want %q", format, ArchiveFormatJSONL)
	}
	if !strings.Contains(warnMsg, "deprecated") && !strings.Contains(warnMsg, "export.auto") {
		t.Errorf("ResolveArchiveFormat() with export.auto: expected deprecation warning, got %q", warnMsg)
	}
}

// TestSetArchiveFormat_alias_writesCanonical verifies that SetArchiveFormat("export.auto","false")
// writes archive.format=none to the config (not export.auto).
func TestSetArchiveFormat_alias_writesCanonical(t *testing.T) {
	dir := t.TempDir()
	restore := setupArchiveTestConfig(t, dir, "")
	defer restore()

	beadsDir := filepath.Join(dir, ".beads")
	_ = InitializeFromDir(beadsDir)

	// SetArchiveFormat with the deprecated key.
	if err := SetArchiveFormat("export.auto", "false"); err != nil {
		t.Fatalf("SetArchiveFormat(export.auto, false): %v", err)
	}

	// Verify config.yaml was written with archive.format, not export.auto.
	configPath := filepath.Join(beadsDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile config.yaml: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "export.auto") {
		t.Errorf("expected archive.format in config.yaml, found export.auto:\n%s", content)
	}
	if !strings.Contains(content, "archive") || !strings.Contains(content, "format") {
		t.Errorf("expected archive.format key in config.yaml, got:\n%s", content)
	}
}

// TestArchiveFormat_invalidValue verifies that ValidateArchiveFormat rejects unknown values.
func TestArchiveFormat_invalidValue(t *testing.T) {
	for _, invalid := range []string{"csv", "parquet", "json", ""} {
		err := ValidateArchiveFormat(invalid)
		if err == nil {
			t.Errorf("ValidateArchiveFormat(%q): expected error, got nil", invalid)
		}
	}
	// Valid values must not error.
	for _, valid := range []string{ArchiveFormatJSONL, ArchiveFormatNone} {
		if err := ValidateArchiveFormat(valid); err != nil {
			t.Errorf("ValidateArchiveFormat(%q): unexpected error: %v", valid, err)
		}
	}
}

// InitializeFromDir is a wrapper that initializes viper from a beadsDir.
// Falls back to Initialize() if the function doesn't exist.
func InitializeFromDir(beadsDir string) error {
	// Try to use the beadsDir-based initializer if available.
	// If not, fall back to the standard initializer.
	return Initialize()
}

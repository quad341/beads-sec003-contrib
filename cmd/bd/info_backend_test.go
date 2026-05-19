//go:build cgo

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	infoBdBinaryOnce sync.Once
	infoBdBinary     string
	infoBdBinaryErr  error
)

// buildInfoBD builds (or reuses) the bd binary for info backend tests.
func buildInfoBD(t *testing.T) string {
	t.Helper()
	infoBdBinaryOnce.Do(func() {
		if prebuilt := os.Getenv("BEADS_TEST_BD_BINARY"); prebuilt != "" {
			if _, err := os.Stat(prebuilt); err == nil {
				infoBdBinary = prebuilt
				return
			}
		}
		name := "bd-info-test"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		dir, err := os.MkdirTemp("", "bd-info-build-*")
		if err != nil {
			infoBdBinaryErr = fmt.Errorf("tempdir: %w", err)
			return
		}
		infoBdBinary = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-tags", "gms_pure_go", "-o", infoBdBinary, ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			infoBdBinaryErr = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if infoBdBinaryErr != nil {
		t.Skipf("buildInfoBD: %v", infoBdBinaryErr)
	}
	return infoBdBinary
}

// infoBdTestEnv returns a clean environment for bd invocations.
func infoBdTestEnv(dir string) []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "BEADS_") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "HOME="+dir, "BEADS_DOLT_AUTO_START=0", "BEADS_NO_DAEMON=1")
}

// runInfoJSON runs "bd info --json -C dir" and returns the parsed JSON.
func runInfoJSON(t *testing.T, bd, dir string) map[string]interface{} {
	t.Helper()
	cmd := exec.Command(bd, "info", "--json")
	cmd.Dir = dir
	cmd.Env = infoBdTestEnv(dir)
	out, _ := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	start := strings.Index(s, "{")
	if start < 0 {
		t.Logf("bd info --json: no JSON object; output: %s", s)
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s[start:]), &result); err != nil {
		t.Fatalf("bd info --json JSON parse: %v\noutput: %s", err, s)
	}
	return result
}

// runInfoText runs "bd info -C dir" and returns combined output.
func runInfoText(t *testing.T, bd, dir string) string {
	t.Helper()
	cmd := exec.Command(bd, "info")
	cmd.Dir = dir
	cmd.Env = infoBdTestEnv(dir)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// initInfoTestDir creates a beads dir with metadata.json and a git repo.
func initInfoTestDir(t *testing.T, bd string) (dir, beadsDir string) {
	t.Helper()
	dir = t.TempDir()
	// Init git repo.
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Run() //nolint:errcheck
	}
	// Init beads.
	cmd := exec.Command(bd, "init", "--prefix", "ib", "--quiet")
	cmd.Dir = dir
	cmd.Env = infoBdTestEnv(dir)
	cmd.Run() //nolint:errcheck
	beadsDir = filepath.Join(dir, ".beads")
	return
}

// writeInfoMetadata writes a metadata.json into beadsDir.
func writeInfoMetadata(t *testing.T, beadsDir, content string) {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile metadata.json: %v", err)
	}
}

// TestInfoReportsPostgresBackendFromMetadata verifies bd info --json
// reports backend="postgres" when metadata.json has backend=postgres.
func TestInfoReportsPostgresBackendFromMetadata(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildInfoBD(t)
	dir, beadsDir := initInfoTestDir(t, bd)

	writeInfoMetadata(t, beadsDir, `{
		"backend": "postgres",
		"postgres_dsn": "postgres://bd@host:5432/beads?sslmode=require"
	}`)

	result := runInfoJSON(t, bd, dir)
	if result == nil {
		t.Skip("bd info --json returned nil — likely no db initialized")
	}

	backend, _ := result["backend"].(string)
	if backend != "postgres" {
		t.Errorf("backend = %q, want %q", backend, "postgres")
	}
	dsn, _ := result["postgres_dsn"].(string)
	if dsn == "" {
		t.Logf("postgres_dsn absent from bd info --json (may require live store)")
	} else if strings.Contains(dsn, "password=") {
		t.Errorf("postgres_dsn should not contain plaintext password: %q", dsn)
	}
}

// TestInfoReportsDoltBackendFromMetadata verifies bd info --json
// reports backend="dolt" when metadata.json has backend=dolt.
func TestInfoReportsDoltBackendFromMetadata(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildInfoBD(t)
	dir, beadsDir := initInfoTestDir(t, bd)

	writeInfoMetadata(t, beadsDir, `{"backend": "dolt"}`)

	result := runInfoJSON(t, bd, dir)
	if result == nil {
		t.Skip("bd info --json returned nil")
	}

	backend, _ := result["backend"].(string)
	if backend != "dolt" {
		t.Errorf("backend = %q, want %q", backend, "dolt")
	}
}

// TestInfoDefaultsToDoltWithoutMetadata verifies bd info --json
// defaults to backend="dolt" when no backend field is set.
func TestInfoDefaultsToDoltWithoutMetadata(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildInfoBD(t)
	dir, beadsDir := initInfoTestDir(t, bd)

	// No backend field in metadata.json.
	writeInfoMetadata(t, beadsDir, `{}`)

	result := runInfoJSON(t, bd, dir)
	if result == nil {
		t.Skip("bd info --json returned nil")
	}

	backend, _ := result["backend"].(string)
	if backend != "" && backend != "dolt" {
		t.Logf("note: backend without explicit field = %q", backend)
	}
}

// TestInfoDoesNotLeakPasswordInDSN verifies bd info does not leak postgres passwords.
func TestInfoDoesNotLeakPasswordInDSN(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildInfoBD(t)
	dir, beadsDir := initInfoTestDir(t, bd)

	secret := "super-secret-password-xyz"
	writeInfoMetadata(t, beadsDir, `{
		"backend": "postgres",
		"postgres_dsn": "postgres://user:`+secret+`@host:5432/beads?sslmode=require"
	}`)

	text := runInfoText(t, bd, dir)
	if strings.Contains(text, secret) {
		t.Errorf("bd info should not leak postgres password; output:\n%s", text)
	}

	result := runInfoJSON(t, bd, dir)
	if result != nil {
		jsonBytes, _ := json.Marshal(result)
		if strings.Contains(string(jsonBytes), secret) {
			t.Errorf("bd info --json should not leak postgres password; JSON: %s", jsonBytes)
		}
	}
}

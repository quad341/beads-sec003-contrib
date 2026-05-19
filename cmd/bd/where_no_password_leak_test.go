//go:build cgo

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoPasswordLeak_WhereJSON verifies that BEADS_POSTGRES_PASSWORD never
// appears in stdout, stderr, or the JSON fields of bd where --json output.
// Uses a synthetic postgres metadata.json — no live Postgres connection needed.
func TestNoPasswordLeak_WhereJSON(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildEmbeddedBD(t)
	dir := t.TempDir()

	// Set up a fake postgres workspace: git repo + .beads/metadata.json.
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	// Stripped DSN (no password field).
	meta := `{
		"backend": "postgres",
		"postgres_dsn": "postgres://pguser@127.0.0.1:5432/testdb?sslmode=disable"
	}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile metadata.json: %v", err)
	}

	secret := "top-secret-pg-password-xyz987"

	// Run bd where --json with BEADS_POSTGRES_PASSWORD set.
	env := bdEnv(dir)
	env = append(env, "BEADS_POSTGRES_PASSWORD="+secret)

	cmd := exec.Command(bd, "where", "--json")
	cmd.Dir = dir
	cmd.Env = env
	// Use CombinedOutput but check the JSON portion of stdout separately.
	combinedOut, _ := cmd.CombinedOutput()
	combined := string(combinedOut)
	// Split on the JSON start if possible.
	stdoutStr := combined
	stderrStr := "" // With CombinedOutput, stderr is merged in

	// Password must NOT appear in stdout or stderr (combined).
	if strings.Contains(stdoutStr, secret) {
		t.Errorf("BEADS_POSTGRES_PASSWORD leaked into output:\n%s", stdoutStr)
	}
	_ = stderrStr

	// Parse JSON and verify no field contains the password.
	var result map[string]interface{}
	if start := strings.Index(stdoutStr, "{"); start >= 0 {
		if err := json.Unmarshal([]byte(stdoutStr[start:]), &result); err == nil {
			checkNoSecretInJSON(t, result, secret, "")
		}
	}
}

// checkNoSecretInJSON recursively checks that no JSON value contains secret.
func checkNoSecretInJSON(t *testing.T, obj interface{}, secret, path string) {
	t.Helper()
	switch v := obj.(type) {
	case map[string]interface{}:
		for k, val := range v {
			checkNoSecretInJSON(t, val, secret, path+"."+k)
		}
	case string:
		if strings.Contains(v, secret) {
			t.Errorf("password leaked in JSON field %s: %q", path, v)
		}
	case []interface{}:
		for i, val := range v {
			checkNoSecretInJSON(t, val, secret, path+"["+string(rune('0'+i))+"]")
		}
	}
}

package main

import (
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realBd and realDolt are resolved once per test binary run, mirroring
// mutation-translator's (be-2cp1d) own fixture convention, so every helper
// below uses the same absolute path regardless of PATH mutations elsewhere
// in a test run.
var (
	realBd   string
	realDolt string
)

func TestMain(m *testing.M) {
	realBd, _ = exec.LookPath("bd")
	realDolt, _ = exec.LookPath("dolt")
	os.Exit(m.Run())
}

func requireBd(t *testing.T) {
	t.Helper()
	if realBd == "" {
		t.Skip("bd not installed, skipping driver-core fixture test")
	}
}

func requireDolt(t *testing.T) {
	t.Helper()
	if realDolt == "" {
		t.Skip("dolt not installed, skipping driver-core fixture test")
	}
}

func runBd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	requireBd(t)
	cmd := exec.Command(realBd, args...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd %s (dir=%s) failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

func runDolt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	requireDolt(t)
	cmd := exec.Command(realDolt, args...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dolt %s (dir=%s) failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

func initBdProject(t *testing.T, name string) string {
	t.Helper()
	return initBdProjectWith(t, name, realBd)
}

func initBdProjectWith(t *testing.T, name, bdBin string) string {
	t.Helper()
	requireBd(t)
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	cmd := exec.Command(bdBin, "init", "--non-interactive", "--role=maintainer")
	cmd.Dir = dir
	cmd.Env = sanitizedEnv(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s init (dir=%s): %v\n%s", bdBin, dir, err, out)
	}
	return dir
}

// dataDir returns the embedded Dolt data directory bd init created under
// dir. filepath.Glob's "*" also matches a sibling ".lock" file, so matches
// must be filtered to directories.
func dataDir(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".beads", "embeddeddolt", "*"))
	if err != nil {
		t.Fatalf("could not find embedded dolt data dir under %s: err=%v", dir, err)
	}
	for _, m := range matches {
		if info, statErr := os.Stat(m); statErr == nil && info.IsDir() {
			return m
		}
	}
	t.Fatalf("could not find embedded dolt data dir under %s: no directory among matches=%v", dir, matches)
	return ""
}

func headCommit(t *testing.T, dir string) string {
	t.Helper()
	out := runDolt(t, dir, "sql", "-q", "SELECT commit_hash FROM dolt_log ORDER BY date DESC LIMIT 1", "-r", "csv")
	rows := parseCSV(t, out)
	if len(rows) < 2 {
		t.Fatalf("dolt_log returned no rows in %s:\n%s", dir, out)
	}
	return rows[1][0]
}

func jsonID(t *testing.T, out string) string {
	t.Helper()
	out = strings.TrimSpace(out)
	const key = `"id"`
	idx := strings.Index(out, key)
	if idx < 0 {
		t.Fatalf("no %q field in bd create output: %s", key, out)
	}
	rest := out[idx+len(key):]
	q1 := strings.Index(rest, `"`)
	if q1 < 0 {
		t.Fatalf("malformed id field in bd create output: %s", out)
	}
	rest = rest[q1+1:]
	q2 := strings.Index(rest, `"`)
	if q2 < 0 {
		t.Fatalf("malformed id field in bd create output: %s", out)
	}
	id := rest[:q2]
	if id == "" {
		t.Fatalf("empty id parsed from bd create output: %s", out)
	}
	return id
}

func parseCSV(t *testing.T, out string) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(out))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v\noutput:\n%s", err, out)
	}
	return rows
}

func hasVar(env []string, name string) bool {
	prefix := name + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

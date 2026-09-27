// Package main implements the Phase 4 oracle-query layer (be-3j80a): a
// read-only Dolt AS OF snapshot reader for the replay-with-oracle harness,
// consumed by the comparator sibling (be-f2gog).
//
// Test-to-acceptance-criterion map (see be-3j80a's exit_contract):
//   - row exists at the queried commit, returned as a plain field-keyed map,
//     ready for JCS canonicalization without translation:
//     TestQueryAsOf_RowExists
//   - row did not exist yet (pre-creation), empty/not-found result:
//     TestQueryAsOf_PreCreation
//   - row after a delete, empty/not-found result: TestQueryAsOf_PostDelete
//   - never opens a write-capable connection / never touches the shared
//     :28231 server (NFR1/NFR2): satisfied by construction (every fixture
//     below is t.TempDir()-scoped, dolt is invoked directly against a local
//     dir, never over a network) and by the guard this layer enforces on its
//     own query path: TestServedDataDir_DetectsLiveServer
//   - env-sanitized entry point, ignores ambient BEADS_DOLT_SERVER_PORT /
//     BEADS_ACTOR (NFR3): TestSanitizedEnv_StripsAmbientVars
//   - queried via `dolt sql -q` with no bind-parameter API, so ref and issue
//     id must be charset-validated before they reach the SQL string:
//     TestQueryAsOf_RejectsInvalidInput
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// realDolt is resolved once, in TestMain, mirroring scripts/corpus-acquire's
// convention so every fixture helper below uses the same absolute path.
var realDolt string

func TestMain(m *testing.M) {
	realDolt, _ = exec.LookPath("dolt")
	os.Exit(m.Run())
}

// requireDolt skips a test that needs the real dolt CLI when none is on PATH.
func requireDolt(t *testing.T) {
	t.Helper()
	if realDolt == "" {
		t.Skip("dolt not installed, skipping oracle-query fixture test")
	}
}

// ---- fixture helpers ----------------------------------------------------

func runDolt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	requireDolt(t)
	cmd := exec.Command(realDolt, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dolt %s (dir=%s) failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

var logHashRE = regexp.MustCompile(`commit\s+([0-9a-v]+)`)

// firstCommitHashFromLog extracts the HEAD commit hash from `dolt log -n 1`
// plain-text output (this dolt version has no --format flag to ask for the
// hash alone).
func firstCommitHashFromLog(t *testing.T, log string) string {
	t.Helper()
	m := logHashRE.FindStringSubmatch(log)
	if m == nil {
		t.Fatalf("could not find a commit hash in dolt log output:\n%s", log)
	}
	return m[1]
}

// queryFixture holds the three commit hashes of a throwaway issues-table
// fixture: before the row is inserted, right after insert, and after delete.
type queryFixture struct {
	dir          string
	beforeInsert string
	afterInsert  string
	afterDelete  string
}

// newQueryFixture creates a throwaway Dolt database (never production, see
// be-3j80a's exit_contract) seeded with one issues row, then deletes it,
// committing after each phase so AS OF queries against the three returned
// hashes can observe the row not-yet-existing, existing, and deleted.
func newQueryFixture(t *testing.T) queryFixture {
	t.Helper()
	requireDolt(t)
	dir := filepath.Join(t.TempDir(), "fixturedb")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	runDolt(t, dir, "init", "--name", "test", "--email", "test@example.com")
	runDolt(t, dir, "sql", "-q", "CREATE TABLE issues (id VARCHAR(64) PRIMARY KEY, title VARCHAR(255), status VARCHAR(32));")
	runDolt(t, dir, "add", "-A")
	runDolt(t, dir, "commit", "-m", "create issues table")
	beforeInsert := firstCommitHashFromLog(t, runDolt(t, dir, "log", "-n", "1"))

	runDolt(t, dir, "sql", "-q", "INSERT INTO issues VALUES ('bd-1', 'Test issue', 'open');")
	runDolt(t, dir, "add", "-A")
	runDolt(t, dir, "commit", "-m", "insert bd-1")
	afterInsert := firstCommitHashFromLog(t, runDolt(t, dir, "log", "-n", "1"))

	runDolt(t, dir, "sql", "-q", "DELETE FROM issues WHERE id = 'bd-1';")
	runDolt(t, dir, "add", "-A")
	runDolt(t, dir, "commit", "-m", "delete bd-1")
	afterDelete := firstCommitHashFromLog(t, runDolt(t, dir, "log", "-n", "1"))

	return queryFixture{dir: dir, beforeInsert: beforeInsert, afterInsert: afterInsert, afterDelete: afterDelete}
}

// ---- QueryAsOf -----------------------------------------------------------

func TestQueryAsOf_RowExists(t *testing.T) {
	requireDolt(t)
	fx := newQueryFixture(t)
	got, err := QueryAsOf(context.Background(), fx.dir, fx.afterInsert, "bd-1")
	if err != nil {
		t.Fatalf("QueryAsOf: %v", err)
	}
	if got == nil {
		t.Fatalf("QueryAsOf returned nil row, want a populated map")
	}
	want := map[string]string{"id": "bd-1", "title": "Test issue", "status": "open"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %q = %q, want %q (full row: %v)", k, got[k], v, got)
		}
	}
}

func TestQueryAsOf_PreCreation(t *testing.T) {
	requireDolt(t)
	fx := newQueryFixture(t)
	got, err := QueryAsOf(context.Background(), fx.dir, fx.beforeInsert, "bd-1")
	if err != nil {
		t.Fatalf("QueryAsOf: %v", err)
	}
	if got != nil {
		t.Fatalf("QueryAsOf returned %v, want nil (row not created yet)", got)
	}
}

func TestQueryAsOf_PostDelete(t *testing.T) {
	requireDolt(t)
	fx := newQueryFixture(t)
	got, err := QueryAsOf(context.Background(), fx.dir, fx.afterDelete, "bd-1")
	if err != nil {
		t.Fatalf("QueryAsOf: %v", err)
	}
	if got != nil {
		t.Fatalf("QueryAsOf returned %v, want nil (row deleted)", got)
	}
}

func TestQueryAsOf_RejectsInvalidInput(t *testing.T) {
	requireDolt(t)
	fx := newQueryFixture(t)
	if _, err := QueryAsOf(context.Background(), fx.dir, "not a ref; DROP TABLE issues", "bd-1"); err == nil {
		t.Fatal("QueryAsOf accepted an unsafe ref, want an error")
	}
	if _, err := QueryAsOf(context.Background(), fx.dir, fx.afterInsert, "bd-1'; DROP TABLE issues; --"); err == nil {
		t.Fatal("QueryAsOf accepted an unsafe issue id, want an error")
	}
}

// ---- safety-guard unit tests (no dolt binary required) -------------------

func TestSanitizedEnv_StripsAmbientVars(t *testing.T) {
	base := []string{
		"BEADS_DOLT_SERVER_PORT=1234",
		"BEADS_DOLT_PORT=1234",
		"BEADS_ACTOR=someone",
		"BD_ACTOR=someone",
		"GT_ROOT=/somewhere",
		"PATH=/usr/bin",
	}
	got := sanitizedEnv(base)
	for _, kv := range got {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		switch name {
		case "BEADS_DOLT_SERVER_PORT", "BEADS_DOLT_PORT", "BEADS_ACTOR", "BD_ACTOR", "GT_ROOT":
			t.Errorf("sanitizedEnv did not strip %s", name)
		}
	}
	found := false
	for _, kv := range got {
		if kv == "PATH=/usr/bin" {
			found = true
		}
	}
	if !found {
		t.Error("sanitizedEnv stripped an unrelated var (PATH)")
	}
}

func TestServedDataDir_DetectsLiveServer(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if got := servedDataDir(child); got != "" {
		t.Errorf("servedDataDir(%s) = %q, want empty (no live server)", child, got)
	}
	served := filepath.Join(root, "a")
	if err := os.MkdirAll(filepath.Join(served, ".dolt"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(served, ".dolt", "sql-server.info"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := servedDataDir(child); got != served {
		t.Errorf("servedDataDir(%s) = %q, want %q", child, got, served)
	}
}

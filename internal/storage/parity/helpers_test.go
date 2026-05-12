//go:build gms_pure_go && (integration_parity || integration_pg)

// Shared helpers for cross-backend parity tests.
//
// Two test files in this package — parity_scenario_test.go (golden UX
// transcript, PG-only, integration_parity tag) and lifecycle_parity_test.go
// (dual-backend lifecycle diff, integration_pg tag) — need the same
// subprocess plumbing, env scrubbing, ID/timestamp normalization, and
// repo-root walk. Hoisted here under a build tag that covers both
// scenarios so neither file has to redefine them.

package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// idPattern normalizes captured bd issue IDs to `<ID>`. Two prefixes are
// observed in parity tests:
//   - "bd-parity-..." used by the legacy PG-only golden scenario.
//   - "parity-..." used by lifecycle parity scenarios (which set
//     `bd init --prefix parity` for both backends).
//
// The expression anchors on either prefix followed by digits and an
// optional suffix so generic words like "create-parent" remain untouched.
var idPattern = regexp.MustCompile(`(?:bd-parity|parity)-\d+(?:-[a-zA-Z0-9]+)?`)

// timestampPattern normalizes RFC3339 timestamps in JSON output. Matches
// any `"<word>_at": "<rfc3339>"` shape — created_at, updated_at,
// started_at, closed_at, etc.
var timestampPattern = regexp.MustCompile(`"\w+_at"\s*:\s*"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})"`)

// uuidPattern normalizes random project IDs / clone IDs that surface in
// bd export.
var uuidPattern = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

// pathPattern normalizes per-test directory paths (always under /tmp).
var pathPattern = regexp.MustCompile(`/tmp/[a-zA-Z0-9._-]+`)

// commitHashPattern normalizes Dolt commit hashes (32 lowercase hex) that
// appear in audit-trail rows. PG has no such field, so without
// normalization the dolt-leg export carries hash strings the PG-leg
// cannot match.
var commitHashPattern = regexp.MustCompile(`\b[0-9a-v]{32}\b`)

// normalizeOutput applies the shared normalization passes so byte
// comparisons across backends/tests are stable. The passes only rewrite
// volatile substrings; surrounding structure is preserved.
func normalizeOutput(raw string) string {
	out := raw
	out = uuidPattern.ReplaceAllString(out, "<UUID>")
	out = timestampPattern.ReplaceAllStringFunc(out, func(match string) string {
		i := strings.Index(match, ":")
		if i < 0 {
			return match
		}
		return match[:i+1] + ` "<TIMESTAMP>"`
	})
	out = pathPattern.ReplaceAllString(out, "<TMPPATH>")
	out = commitHashPattern.ReplaceAllString(out, "<COMMIT>")
	out = idPattern.ReplaceAllString(out, "<ID>")
	return out
}

// runBDEnv invokes bd with args + caller-supplied env additions.  The
// environment scrubs BEADS_DIR (so subprocess uses the supplied dir) and
// every identity-bearing var so output stays byte-identical across
// rigs/CI/dev. Callers pass deterministic identity values via extraEnv.
func runBDEnv(bd, dir string, extraEnv, args []string) (string, string, error) {
	cmd := exec.Command(bd, args...)
	cmd.Dir = dir
	scrub := []string{
		"BEADS_DIR",
		"BEADS_ACTOR",
		"BD_ACTOR",
		"GIT_AUTHOR_EMAIL",
		"GIT_AUTHOR_NAME",
		"GIT_COMMITTER_EMAIL",
		"GIT_COMMITTER_NAME",
		"USER",
		"LOGNAME",
		"EMAIL",
	}
	cmd.Env = append(filterEnv(os.Environ(), scrub...), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// extractIDFromJSON pulls the "id" field from a bd JSON response. Handles
// both single-object responses ({"id": ...}) and array responses (bd
// create can return either shape).
func extractIDFromJSON(stdout string) (string, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", fmt.Errorf("empty stdout")
	}
	var single struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &single); err == nil && single.ID != "" {
		return single.ID, nil
	}
	var arr []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &arr); err == nil && len(arr) > 0 && arr[0].ID != "" {
		return arr[0].ID, nil
	}
	return "", fmt.Errorf("no id in JSON: %s", truncate(stdout, 200))
}

// extractPasswordFromDSN returns the password component of a postgres URI
// or keyword DSN, or "" if absent. bd init strips the password from the
// stored metadata; subprocess invocations need BEADS_POSTGRES_PASSWORD set
// to reconnect. Delegating to pgconn.ParseConfig keeps URL decoding and
// quoted-keyword DSNs correct.
func extractPasswordFromDSN(rawDSN string) string {
	cfg, err := pgconn.ParseConfig(rawDSN)
	if err != nil {
		return ""
	}
	return cfg.Password
}

func filterEnv(env []string, drop ...string) []string {
	out := make([]string, 0, len(env))
outer:
	for _, kv := range env {
		for _, key := range drop {
			if strings.HasPrefix(kv, key+"=") {
				continue outer
			}
		}
		out = append(out, kv)
	}
	return out
}

func isolatedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "bd-parity-")
	if err != nil {
		t.Fatalf("isolatedTempDir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// buildBD compiles ./bd at the repo root once per test binary and returns
// its path. Subsequent calls reuse the existing artifact.
func buildBD(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	bd := filepath.Join(root, "bd")
	if info, err := os.Stat(bd); err == nil && info.Size() > 0 {
		return bd
	}
	cmd := exec.Command("go", "build", "-tags", "gms_pure_go", "-o", bd, "./cmd/bd/")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build bd: %v\n%s", err, out)
	}
	return bd
}

// repoRoot walks up from the test's working directory until it finds
// go.mod. Used to locate the bd binary and other repo-anchored paths.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod above %s", dir)
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...<truncated>"
}

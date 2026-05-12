//go:build gms_pure_go && integration_parity

// Package parity hosts cross-backend UX parity tests. It is intentionally
// distinct from internal/storage/postgres (which exercises the driver
// in isolation) and cmd/bd init tests (which exercise initialization
// against one backend at a time) — its job is to assert that the bd
// CLI surface presents the same user-visible output regardless of
// which backend serves it.
//
// The test is gated by build tag `integration_parity` per ADR be-l7t.6
// (FR-8). Activated by both legs of the CI matrix; skipped when neither
// backend can be reached (e.g. local dev without a Docker socket).
package parity

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/postgres/testfixture"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the parity golden fixture from a successful run (PG only)")

// scenarioStep describes one bd CLI invocation in the parity scenario.
// args is interpolated against the scenario state (see runScenario for
// the variable substitution scheme): "{ID1}" expands to the first
// captured issue ID, "{ID2}" to the second, etc.
type scenarioStep struct {
	name string
	args []string
	// expectExit is the expected process exit code. 0 unless explicitly set.
	expectExit int
	// captureID, when non-zero, parses the resulting JSON output and stashes
	// the issue ID at scenarioState.ids[captureID-1] for later interpolation.
	captureID int
	// json controls whether stdout is parsed as JSON for ID extraction.
	json bool
	// extraEnv holds step-scoped environment additions appended to the
	// scenario-wide extraEnv. Used to exercise env-driven gates (e.g.
	// BD_BACKUP_ENABLED=true to trigger the be-xz4 PostRun path).
	extraEnv []string
}

// scenario is the canonical bd CLI sequence exercised in both legs.
// Order matters; later steps reference IDs captured by earlier steps.
//
// The "backup-status-fresh" and "auto-backup-trigger" steps exercise
// the be-xz4 gating sweep. Without the gates, `bd create` with
// BD_BACKUP_ENABLED=true fatal-exits on PG with "Dolt backend required"
// from PersistentPostRun → maybeAutoBackup → dVC.GetCurrentCommit.
// The auto-backup-trigger step is invoked with backup.enabled set via
// the environment (see runParityScenario) so no on-disk config.yaml is
// required.
var scenario = []scenarioStep{
	{name: "create-parent", args: []string{"create", "Parent issue", "-t", "task", "-p", "1", "--json"}, captureID: 1, json: true},
	{name: "create-child", args: []string{"create", "Child issue", "-t", "task", "-p", "2", "--json"}, captureID: 2, json: true},
	{name: "dep-add", args: []string{"dep", "add", "{ID2}", "{ID1}", "--type", "blocks"}},
	{name: "ready", args: []string{"ready", "--json"}},
	{name: "claim-parent", args: []string{"update", "{ID1}", "--claim", "--assignee", "tester", "--json"}},
	{name: "comment", args: []string{"comment", "{ID1}", "parity scenario comment"}},
	{name: "close-parent", args: []string{"close", "{ID1}", "--reason", "scenario complete"}},
	{name: "export", args: []string{"export"}},
	{name: "backup-status-fresh", args: []string{"backup", "status", "--json"}, extraEnv: []string{"BD_BACKUP_ENABLED=true"}},
	{name: "auto-backup-trigger", args: []string{"create", "Post-backup-enable issue", "-t", "task", "-p", "3", "--json"}, captureID: 3, json: true, extraEnv: []string{"BD_BACKUP_ENABLED=true"}},
}

// scenarioState carries values captured during the scenario for use in
// later interpolation steps.
type scenarioState struct {
	ids []string
}

// goldenFile names the path to the committed reference output, relative
// to this test package's directory (testdata/ co-located with the test).
const goldenFile = "internal/storage/parity/testdata/parity_scenario.golden.txt"

func TestUXParity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("parity scenario runs Unix-shell-style subprocesses; not portable to Windows in v1")
	}
	bd := buildBD(t)

	got, err := runParityScenario(t, bd)
	if err != nil {
		t.Fatalf("scenario: %v", err)
	}

	normalized := normalizeOutput(got)

	if *updateGolden {
		if err := os.WriteFile(filepath.Join(repoRoot(t), goldenFile), []byte(normalized), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("Updated golden: %s", goldenFile)
		return
	}

	want, err := os.ReadFile(filepath.Join(repoRoot(t), goldenFile))
	if err != nil {
		t.Fatalf("read golden: %v\n  hint: run with -update-golden after a known-good PG run", err)
	}

	if string(want) != normalized {
		t.Errorf("parity scenario diverged from golden\n--- want\n%s\n--- got\n%s",
			truncate(string(want), 2000), truncate(normalized, 2000))
	}
}

// runParityScenario executes the canonical bd CLI sequence against a
// freshly-initialized PG-backed bd, captures stdout for each step, and
// returns the concatenated transcript.
func runParityScenario(t *testing.T, bd string) (string, error) {
	t.Helper()
	rawDSN := testfixture.ForTest(t)
	dir := isolatedTempDir(t)

	// Extract password from raw DSN so subsequent (post-init) runs can
	// authenticate via BEADS_POSTGRES_PASSWORD. metadata.json carries the
	// stripped form by design (see store_factory.go).
	password := extractPasswordFromDSN(rawDSN)
	t.Logf("rawDSN=%q password=<redacted>", rawDSN)
	// Deterministic identity: parity goldens must be byte-identical across
	// rigs/CI/dev. The bd CLI derives created_by/owner/author from
	// BEADS_ACTOR, BD_ACTOR, GIT_AUTHOR_EMAIL, etc. Inheriting them from the
	// caller's shell contaminates the golden — see runBDEnv where these are
	// scrubbed from the inherited environment.
	extraEnv := []string{
		"BEADS_DOLT_AUTO_START=0",
		"BEADS_ACTOR=parity-author",
		"BD_ACTOR=parity-author",
		"GIT_AUTHOR_EMAIL=parity@bd.test",
		"GIT_AUTHOR_NAME=parity-author",
		"GIT_COMMITTER_EMAIL=parity@bd.test",
		"GIT_COMMITTER_NAME=parity-author",
	}
	if password != "" {
		extraEnv = append(extraEnv, "BEADS_POSTGRES_PASSWORD="+password)
	}

	state := &scenarioState{}
	var buf bytes.Buffer

	initStdout, initStderr, initErr := runBDEnv(bd, dir, extraEnv, []string{"init", "--backend", "postgres", "--dsn", rawDSN, "--quiet"})
	if initErr != nil {
		return "", fmt.Errorf("init: %w\nstdout: %s\nstderr: %s", initErr, initStdout, initStderr)
	}
	t.Logf("init stdout=%q stderr=%q", initStdout, initStderr)
	if entries, err := os.ReadDir(filepath.Join(dir, ".beads")); err != nil {
		t.Logf(".beads dir missing after init: %v", err)
	} else {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Logf(".beads contents: %v", names)
	}

	for _, step := range scenario {
		args := interpolate(step.args, state)
		stepEnv := extraEnv
		if len(step.extraEnv) > 0 {
			stepEnv = append(append([]string{}, extraEnv...), step.extraEnv...)
		}
		stdout, stderr, err := runBDEnv(bd, dir, stepEnv, args)
		exit := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exit = exitErr.ExitCode()
			} else {
				return "", fmt.Errorf("step %s: %w", step.name, err)
			}
		}

		if exit != step.expectExit {
			return "", fmt.Errorf("step %s: exit=%d want=%d\nstdout: %s\nstderr: %s",
				step.name, exit, step.expectExit, stdout, stderr)
		}

		fmt.Fprintf(&buf, "### step: %s\n", step.name)
		fmt.Fprintf(&buf, "$ bd %s\n", strings.Join(args, " "))
		fmt.Fprintf(&buf, "exit: %d\n", exit)
		fmt.Fprintf(&buf, "stdout:\n%s\n", stdout)
		fmt.Fprintf(&buf, "stderr:\n%s\n\n", stderr)

		if step.captureID > 0 && step.json {
			id, err := extractIDFromJSON(stdout)
			if err != nil {
				return "", fmt.Errorf("step %s: extract id: %w\nraw stdout: %s", step.name, err, stdout)
			}
			for len(state.ids) < step.captureID {
				state.ids = append(state.ids, "")
			}
			state.ids[step.captureID-1] = id
		}
	}

	return buf.String(), nil
}

// interpolate substitutes "{IDN}" placeholders in args with the captured
// scenarioState.ids[N-1] value.
func interpolate(args []string, state *scenarioState) []string {
	out := make([]string, len(args))
	for i, a := range args {
		for j, id := range state.ids {
			a = strings.ReplaceAll(a, fmt.Sprintf("{ID%d}", j+1), id)
		}
		out[i] = a
	}
	return out
}

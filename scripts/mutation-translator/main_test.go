// Package main implements the Phase 4 mutation-kind translator (be-2cp1d):
// given a historical dolt_log commit's row-diff for one issue, produces and
// executes the equivalent `bd` CLI invocation against a working clone.
//
// Test-to-acceptance-criterion map (see be-2cp1d's acceptance_criteria):
//
//	AC1 (for each of the 7 mutation kinds, given a commit's diff for one
//	     issue, produce+execute the equivalent bd CLI invocation against a
//	     working clone -- never a hand-written row insert)
//	  -> TestClassifyAndExecute_Create
//	  -> TestClassifyAndExecute_Update
//	  -> TestClassifyAndExecute_Delete
//	  -> TestClassifyAndExecute_Merge
//	  -> TestClassifyAndExecute_Import
//	  -> TestClassifyAndExecute_DepAdd
//	  -> TestClassifyAndExecute_DepRemove
//
//	AC2 (no-op commits -- changed nothing material for the sampled issue --
//	     detected and translate to zero replayed mutations, not a spurious
//	     version)
//	  -> TestClassify_DepAddAutoCommitIsNoop (is_blocked-only row change)
//	  -> TestClassify_DepRemoveAutoCommitIsNoop (symmetric)
//
//	AC3 (dep_add/dep_remove actually exercise dependency-edge versioning;
//	     an incomplete fix must produce a real, surfaced mismatch, not a
//	     silent skip)
//	  -> TestClassifyAndExecute_DepAdd (asserts the edge is actually
//	     replayed, not swallowed as part of the paired auto-commit)
//	  -> TestClassifyAndExecute_DepRemove
//	  -> TestClassify_UnsupportedFieldChangeSurfacesError (the general
//	     "never silently skip a real row change" principle applied beyond
//	     dep_add/dep_remove: a changed column this translator has no bd flag
//	     mapping for is a hard error, not a dropped mutation)
//
//	AC4 (fixture-based coverage per mutation kind, against a small local
//	     fixture clone, never production; asserts both the bd invocation
//	     made and the resulting working-clone end state)
//	  -> all TestClassifyAndExecute_* tests build their fixtures via
//	     newMutationFixture (a throwaway t.TempDir()-scoped bd project,
//	     ambient BEADS_DIR/BEADS_DOLT_SERVER_PORT/etc. stripped by
//	     sanitizedEnv before any bd/dolt child process starts -- see
//	     TestSanitizedEnv_StripsAmbientVars) and assert on both Classify's
//	     returned Action(s) and a post-Execute read-back from the work clone.
package main

import (
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realBd and realDolt are resolved once, mirroring scripts/oracle-query's
// realDolt convention, so every fixture helper below uses the same absolute
// path regardless of PATH mutations elsewhere in a test run.
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
		t.Skip("bd not installed, skipping mutation-translator fixture test")
	}
}

func requireDolt(t *testing.T) {
	t.Helper()
	if realDolt == "" {
		t.Skip("dolt not installed, skipping mutation-translator fixture test")
	}
}

// ---- fixture helpers ------------------------------------------------------

// runBd runs the real bd CLI in dir with a sanitized environment -- the same
// sanitizedEnv main.go's own Execute uses, so fixture setup is guarded by
// the exact mechanism under test, not a parallel hand-rolled one.
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

// initBdProject creates a fresh, throwaway embedded-Dolt bd project in its
// own t.TempDir() subdirectory -- never the shared production-adjacent store
// (NFR1/NFR2; see cairn beads-testdb-production-leak). Isolation depends
// entirely on sanitizedEnv actually stripping BEADS_DIR and friends before
// bd resolves which store to operate on: runBd is the only way this helper
// invokes bd.
func initBdProject(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	runBd(t, dir, "init", "--non-interactive", "--role=maintainer")
	return dir
}

// dataDir returns the embedded Dolt data directory bd init created under
// dir, where the actual dolt_log/dolt_commit_diff_* system tables live.
func dataDir(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".beads", "embeddeddolt", "*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("could not find embedded dolt data dir under %s: err=%v matches=%v", dir, err, matches)
	}
	return matches[0]
}

// headCommit returns the current HEAD commit hash in dir (a dataDir), via
// dolt_log rather than `dolt log` text parsing -- CSV columns are stable
// where plain-text log formatting is not guaranteed across dolt versions.
func headCommit(t *testing.T, dir string) string {
	t.Helper()
	out := runDolt(t, dir, "sql", "-q", "SELECT commit_hash FROM dolt_log ORDER BY date DESC LIMIT 1", "-r", "csv")
	rows := parseCSV(t, out)
	if len(rows) < 2 {
		t.Fatalf("dolt_log returned no rows in %s:\n%s", dir, out)
	}
	return rows[1][0]
}

// jsonID extracts the created issue id from `bd create --json` output,
// tolerating both the single-object and array response shapes observed
// empirically across bd versions (see be-2cp1d bead notes).
func jsonID(t *testing.T, out string) string {
	t.Helper()
	out = strings.TrimSpace(out)
	var id string
	// Minimal extraction: avoid a JSON import purely for one field, matching
	// this package's existing dependency footprint (stdlib only, like both
	// siblings). The id is always the first `"id": "..."` occurrence in
	// either shape.
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
	id = rest[:q2]
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

// mutationFixture pairs a source project (where historical commits are
// authored and read back via Classify) with a work project (a fresh bd
// project any Execute call replays into) -- matching the harness's own
// oracle-clone vs. working-clone separation (be-hs42e.5.2 §8) at test scale.
type mutationFixture struct {
	sourceDir string // project root (bd init'd), not the dolt data dir
	source    string // dolt data dir under sourceDir, for dolt_log queries
	workDir   string // project root for a fresh, independent replay target
}

func newMutationFixture(t *testing.T) *mutationFixture {
	t.Helper()
	requireBd(t)
	requireDolt(t)
	sourceDir := initBdProject(t, "source")
	workDir := initBdProject(t, "work")
	return &mutationFixture{
		sourceDir: sourceDir,
		source:    dataDir(t, sourceDir),
		workDir:   workDir,
	}
}

// bdShowField returns one field's value from `bd show <id> --json` in dir,
// used to confirm a work clone's post-Execute end state without re-deriving
// bd's own JSON shape more than this one field needs.
func bdShowField(t *testing.T, dir, id, field string) string {
	t.Helper()
	out := runBd(t, dir, "show", id, "--json")
	key := `"` + field + `"`
	idx := strings.Index(out, key)
	if idx < 0 {
		return "" // absent field (e.g. issue does not exist) reads as empty
	}
	rest := out[idx+len(key):]
	c := strings.Index(rest, ":")
	if c < 0 {
		t.Fatalf("malformed field %q in bd show output: %s", field, out)
	}
	rest = strings.TrimSpace(rest[c+1:])
	if !strings.HasPrefix(rest, `"`) {
		// Non-string field (bool/number) -- read up to the next comma/brace.
		end := strings.IndexAny(rest, ",}")
		if end < 0 {
			end = len(rest)
		}
		return strings.TrimSpace(rest[:end])
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatalf("malformed string field %q in bd show output: %s", field, out)
	}
	return rest[:end]
}

// issueExists reports whether bd show succeeds for id in dir.
func issueExists(t *testing.T, dir, id string) bool {
	t.Helper()
	requireBd(t)
	cmd := exec.Command(realBd, "show", id, "--json")
	cmd.Dir = dir
	cmd.Env = sanitizedEnv(os.Environ())
	err := cmd.Run()
	return err == nil
}

// depEdgeExists reports whether a dependency row issueID -> dependsOnID
// exists in dir's work project.
func depEdgeExists(t *testing.T, dir, issueID, dependsOnID string) bool {
	t.Helper()
	out := runBd(t, dir, "show", issueID, "--json")
	return strings.Contains(out, dependsOnID)
}

// ---- Classify: per-mutation-kind fixtures + execution ---------------------

func TestClassifyAndExecute_Create(t *testing.T) {
	fx := newMutationFixture(t)
	from := headCommit(t, fx.source)
	out := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	id := jsonID(t, out)
	to := headCommit(t, fx.source)

	actions, err := Classify(context.Background(), fx.source, from, to, id)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindCreate {
		t.Fatalf("Classify actions = %+v, want exactly one KindCreate action", actions)
	}
	if !containsAll(actions[0].Argv, "create", "--id", id, "--description", "desc A") {
		t.Errorf("create argv = %v, missing expected id/description", actions[0].Argv)
	}

	applyActions(t, fx.workDir, actions)
	if !issueExists(t, fx.workDir, id) {
		t.Fatalf("issue %s does not exist in work clone after Execute", id)
	}
	if got := bdShowField(t, fx.workDir, id, "description"); got != "desc A" {
		t.Errorf("work clone description = %q, want %q", got, "desc A")
	}
}

func TestClassifyAndExecute_Update(t *testing.T) {
	fx := newMutationFixture(t)
	out := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	id := jsonID(t, out)

	// Seed the work clone with the pre-update state so Execute has
	// something to update -- a real replay would have already applied the
	// create commit that precedes this one.
	createActions := mustClassifyHead(t, fx, id)
	applyActions(t, fx.workDir, createActions)

	from := headCommit(t, fx.source)
	runBd(t, fx.sourceDir, "update", id, "--description", "desc A updated")
	to := headCommit(t, fx.source)

	actions, err := Classify(context.Background(), fx.source, from, to, id)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindUpdate {
		t.Fatalf("Classify actions = %+v, want exactly one KindUpdate action", actions)
	}
	if !containsAll(actions[0].Argv, "update", id, "--description", "desc A updated") {
		t.Errorf("update argv = %v, missing expected description flag", actions[0].Argv)
	}

	applyActions(t, fx.workDir, actions)
	if got := bdShowField(t, fx.workDir, id, "description"); got != "desc A updated" {
		t.Errorf("work clone description after update = %q, want %q", got, "desc A updated")
	}
}

func TestClassifyAndExecute_Delete(t *testing.T) {
	fx := newMutationFixture(t)
	out := runBd(t, fx.sourceDir, "create", "Widget C", "--description", "desc C", "--type", "task", "--json")
	id := jsonID(t, out)
	applyActions(t, fx.workDir, mustClassifyHead(t, fx, id))

	from := headCommit(t, fx.source)
	runBd(t, fx.sourceDir, "delete", id, "--force")
	to := headCommit(t, fx.source)

	actions, err := Classify(context.Background(), fx.source, from, to, id)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindDelete {
		t.Fatalf("Classify actions = %+v, want exactly one KindDelete action", actions)
	}
	if !containsAll(actions[0].Argv, "delete", id, "--force") {
		t.Errorf("delete argv = %v, missing expected id/--force", actions[0].Argv)
	}

	applyActions(t, fx.workDir, actions)
	if issueExists(t, fx.workDir, id) {
		t.Fatalf("issue %s still exists in work clone after replaying delete", id)
	}
}

func TestClassifyAndExecute_DepAdd(t *testing.T) {
	fx := newMutationFixture(t)
	outA := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	a := jsonID(t, outA)
	outB := runBd(t, fx.sourceDir, "create", "Widget B", "--description", "desc B", "--type", "task", "--json")
	b := jsonID(t, outB)
	applyActions(t, fx.workDir, mustClassifyHead(t, fx, a))
	applyActions(t, fx.workDir, mustClassifyHead(t, fx, b))

	from := headCommit(t, fx.source)
	runBd(t, fx.sourceDir, "dep", "add", a, b)
	to := headCommit(t, fx.source)

	// The primary commit (dependencies row added) must classify as a real
	// dep_add -- this is the R3 must-not-silently-skip scenario (AC3).
	actions, err := Classify(context.Background(), fx.source, from, to, a)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindDepAdd {
		t.Fatalf("Classify actions = %+v, want exactly one KindDepAdd action", actions)
	}
	if !containsAll(actions[0].Argv, "dep", "add", a, b) {
		t.Errorf("dep_add argv = %v, missing expected issue ids", actions[0].Argv)
	}

	applyActions(t, fx.workDir, actions)
	if !depEdgeExists(t, fx.workDir, a, b) {
		t.Fatalf("dependency %s -> %s does not exist in work clone after Execute", a, b)
	}
}

func TestClassify_DepAddAutoCommitIsNoop(t *testing.T) {
	fx := newMutationFixture(t)
	outA := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	a := jsonID(t, outA)
	outB := runBd(t, fx.sourceDir, "create", "Widget B", "--description", "desc B", "--type", "task", "--json")
	b := jsonID(t, outB)

	runBd(t, fx.sourceDir, "dep", "add", a, b)

	// dep add mints TWO commits: a primary (dependencies row added) and an
	// auto-commit companion (issues row modified, is_blocked only). Find
	// the auto-commit's own from/to pair via dolt_log rather than assuming
	// the current HEAD IS it -- HEAD is whichever of the two lands last.
	rows := parseCSV(t, runDolt(t, fx.source, "sql", "-q",
		"SELECT commit_hash, message FROM dolt_log ORDER BY date ASC", "-r", "csv"))
	autoCommit, autoParent := findAutoCommit(t, rows, "dep add")

	actions, err := Classify(context.Background(), fx.source, autoParent, autoCommit, a)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindNoop {
		t.Fatalf("Classify actions for is_blocked-only auto-commit = %+v, want exactly one KindNoop action", actions)
	}
}

func TestClassifyAndExecute_DepRemove(t *testing.T) {
	fx := newMutationFixture(t)
	outA := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	a := jsonID(t, outA)
	outB := runBd(t, fx.sourceDir, "create", "Widget B", "--description", "desc B", "--type", "task", "--json")
	b := jsonID(t, outB)
	runBd(t, fx.sourceDir, "dep", "add", a, b)

	applyActions(t, fx.workDir, mustClassifyHead(t, fx, a))
	applyActions(t, fx.workDir, mustClassifyHead(t, fx, b))
	runBd(t, fx.workDir, "dep", "add", a, b)

	from := headCommit(t, fx.source)
	runBd(t, fx.sourceDir, "dep", "remove", a, b)
	to := headCommit(t, fx.source)

	actions, err := Classify(context.Background(), fx.source, from, to, a)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindDepRemove {
		t.Fatalf("Classify actions = %+v, want exactly one KindDepRemove action", actions)
	}
	if !containsAll(actions[0].Argv, "dep", "remove", a, b) {
		t.Errorf("dep_remove argv = %v, missing expected issue ids", actions[0].Argv)
	}

	applyActions(t, fx.workDir, actions)
	if depEdgeExists(t, fx.workDir, a, b) {
		t.Fatalf("dependency %s -> %s still exists in work clone after replaying dep_remove", a, b)
	}
}

func TestClassify_DepRemoveAutoCommitIsNoop(t *testing.T) {
	fx := newMutationFixture(t)
	outA := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	a := jsonID(t, outA)
	outB := runBd(t, fx.sourceDir, "create", "Widget B", "--description", "desc B", "--type", "task", "--json")
	b := jsonID(t, outB)
	runBd(t, fx.sourceDir, "dep", "add", a, b)
	runBd(t, fx.sourceDir, "dep", "remove", a, b)

	rows := parseCSV(t, runDolt(t, fx.source, "sql", "-q",
		"SELECT commit_hash, message FROM dolt_log ORDER BY date ASC", "-r", "csv"))
	autoCommit, autoParent := findAutoCommit(t, rows, "dep remove")

	actions, err := Classify(context.Background(), fx.source, autoParent, autoCommit, a)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindNoop {
		t.Fatalf("Classify actions for is_blocked-only auto-commit = %+v, want exactly one KindNoop action", actions)
	}
}

func TestClassifyAndExecute_Merge(t *testing.T) {
	fx := newMutationFixture(t)
	outD := runBd(t, fx.sourceDir, "create", "Duplicate Widget", "--description", "identical desc", "--type", "task", "--json")
	d := jsonID(t, outD)
	outE := runBd(t, fx.sourceDir, "create", "Duplicate Widget", "--description", "identical desc", "--type", "task", "--json")
	e := jsonID(t, outE)
	applyActions(t, fx.workDir, mustClassifyHead(t, fx, d))
	applyActions(t, fx.workDir, mustClassifyHead(t, fx, e))

	from := headCommit(t, fx.source)
	runBd(t, fx.sourceDir, "duplicates", "--auto-merge")
	to := headCommit(t, fx.source)

	// bd's auto-merge tie-break picks the lexicographically smallest
	// hash-based ID as the surviving target (internal/idgen.GenerateHashID).
	// d and e share identical title/description/creator, so their hash
	// suffixes differ only by creation timestamp and are uncorrelated with
	// creation order -- empirically confirmed via 10 trials of this exact
	// fixture: 5 closed the first-created issue, 5 closed the second (see
	// be-2cp1d bead notes). Discover which one actually closed instead of
	// assuming e, or this assertion is flaky by construction.
	closedID, survivorID := d, e
	if bdShowField(t, fx.sourceDir, e, "status") == "closed" {
		closedID, survivorID = e, d
	}

	// A merge commit spans two tables in one dolt_log commit (verified
	// empirically: dolt_diff shows both `issues` and `dependencies` changed
	// for the same commit_hash) -- Classify must not special-case "merge" as
	// its own kind; it falls out of the same per-table rules as a plain
	// close plus a plain dep_add, decomposed into two Actions.
	actions, err := Classify(context.Background(), fx.source, from, to, closedID)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("Classify actions = %+v, want exactly two actions (close + dep_add)", actions)
	}
	var sawClose, sawDepAdd bool
	for _, a := range actions {
		switch a.Kind {
		case KindClose:
			sawClose = true
			if !containsAll(a.Argv, "close", closedID) {
				t.Errorf("merge close argv = %v, want id %s", a.Argv, closedID)
			}
		case KindDepAdd:
			sawDepAdd = true
			if !containsAll(a.Argv, "dep", "add", closedID, survivorID) {
				t.Errorf("merge dep_add argv = %v, want edge %s -> %s", a.Argv, closedID, survivorID)
			}
		}
	}
	if !sawClose || !sawDepAdd {
		t.Fatalf("Classify actions = %+v, want one KindClose and one KindDepAdd", actions)
	}

	applyActions(t, fx.workDir, actions)
	if got := bdShowField(t, fx.workDir, closedID, "status"); got != "closed" {
		t.Errorf("work clone status for %s = %q, want %q", closedID, got, "closed")
	}
	if !depEdgeExists(t, fx.workDir, closedID, survivorID) {
		t.Fatalf("dependency %s -> %s does not exist in work clone after replaying merge", closedID, survivorID)
	}
}

func TestClassifyAndExecute_Import(t *testing.T) {
	fx := newMutationFixture(t)
	from := headCommit(t, fx.source)

	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	const id = "fixture-import-f"
	line := `{"id":"` + id + `","title":"Imported Widget F","description":"desc F","issue_type":"task","status":"open","priority":2}`
	if err := os.WriteFile(importFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runBd(t, fx.sourceDir, "import", importFile)
	to := headCommit(t, fx.source)

	// Import is not a distinct classification primitive: a newly-imported
	// issue's row diff is `added`, indistinguishable from (and replayed
	// identically to) a plain bd create.
	actions, err := Classify(context.Background(), fx.source, from, to, id)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != KindCreate {
		t.Fatalf("Classify actions = %+v, want exactly one KindCreate action", actions)
	}
	if !containsAll(actions[0].Argv, "create", "--id", id) {
		t.Errorf("import-as-create argv = %v, missing expected --id", actions[0].Argv)
	}

	applyActions(t, fx.workDir, actions)
	if !issueExists(t, fx.workDir, id) {
		t.Fatalf("issue %s does not exist in work clone after replaying import", id)
	}
}

func TestClassify_UnsupportedFieldChangeSurfacesError(t *testing.T) {
	fx := newMutationFixture(t)
	out := runBd(t, fx.sourceDir, "create", "Widget A", "--description", "desc A", "--type", "task", "--json")
	id := jsonID(t, out)

	from := headCommit(t, fx.source)
	// owner has no bd update flag (confirmed via bd update --help): a
	// historical commit that changed only this column must surface as an
	// error, not silently vanish as an unrecognized no-op.
	runDolt(t, fx.source, "sql", "-q", "UPDATE issues SET owner = 'someone-else' WHERE id = '"+id+"'")
	runDolt(t, fx.source, "add", "-A")
	runDolt(t, fx.source, "commit", "-m", "test: direct owner mutation")
	to := headCommit(t, fx.source)

	_, err := Classify(context.Background(), fx.source, from, to, id)
	if err == nil {
		t.Fatal("Classify silently accepted an unmapped field change, want an error")
	}
}

func TestSanitizedEnv_StripsAmbientVars(t *testing.T) {
	base := []string{
		"BEADS_DOLT_SERVER_PORT=1234",
		"BEADS_DOLT_PORT=1234",
		"BEADS_ACTOR=someone",
		"BD_ACTOR=someone",
		"GT_ROOT=/somewhere",
		"BEADS_DIR=/home/jaword/projects/beads/.beads",
		"BEADS_HOLDER_TOKEN=tok",
		"GC_BEADS_SCOPE_ROOT=/somewhere",
		"BEADS_DOLT_AUTO_START=1",
		"BEADS_DOLT_SYNC_CLI_REMOTES=1",
		"BEADS_BACKUP_ENABLED=1",
		"PATH=/usr/bin",
	}
	got := sanitizedEnv(base)
	stripped := map[string]bool{
		"BEADS_DOLT_SERVER_PORT": true, "BEADS_DOLT_PORT": true, "BEADS_ACTOR": true,
		"BD_ACTOR": true, "GT_ROOT": true, "BEADS_DIR": true, "BEADS_HOLDER_TOKEN": true,
		"GC_BEADS_SCOPE_ROOT": true, "BEADS_DOLT_AUTO_START": true,
		"BEADS_DOLT_SYNC_CLI_REMOTES": true, "BEADS_BACKUP_ENABLED": true,
	}
	for _, kv := range got {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if stripped[name] {
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

// ---- shared helpers used across the tests above ---------------------------

// mustClassifyHead classifies the full history for id, from the very first
// dolt_log commit through HEAD, so callers can seed a work clone's pre-state
// (e.g. issue B's creation) before exercising the transition they actually
// care about (e.g. a dep_add between A and B).
func mustClassifyHead(t *testing.T, fx *mutationFixture, id string) []Action {
	t.Helper()
	rows := parseCSV(t, runDolt(t, fx.source, "sql", "-q",
		"SELECT commit_hash FROM dolt_log ORDER BY date ASC LIMIT 1", "-r", "csv"))
	if len(rows) < 2 {
		t.Fatalf("dolt_log has no commits in %s", fx.source)
	}
	root := rows[1][0]
	to := headCommit(t, fx.source)
	actions, err := Classify(context.Background(), fx.source, root, to, id)
	if err != nil {
		t.Fatalf("mustClassifyHead: Classify(%s): %v", id, err)
	}
	return actions
}

func applyActions(t *testing.T, workDir string, actions []Action) {
	t.Helper()
	for _, a := range actions {
		if err := Execute(context.Background(), workDir, a); err != nil {
			t.Fatalf("Execute(%+v): %v", a, err)
		}
	}
}

func containsAll(argv []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, a := range argv {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// findAutoCommit scans an ordered (oldest-first) dolt_log [commit_hash,
// message] CSV for the "(auto-commit)" companion of a bd dep add/remove
// operation, identified by messageSubstr (e.g. "dep add"), and returns its
// hash plus its immediate predecessor's hash as the from/to pair to
// classify. Two dolt_log commits share every dep-add/dep-remove operation
// (verified empirically): a primary (dependencies row) followed immediately
// by this auto-commit (issues row, is_blocked only) -- the predecessor in
// log order is always the primary, never an unrelated commit, because bd
// commits the pair back to back with nothing interleaved.
func findAutoCommit(t *testing.T, rows [][]string, messageSubstr string) (commit, parent string) {
	t.Helper()
	if len(rows) < 2 {
		t.Fatalf("dolt_log returned no rows")
	}
	for i := 1; i < len(rows); i++ {
		hash, msg := rows[i][0], rows[i][1]
		if strings.Contains(msg, messageSubstr) && strings.Contains(msg, "(auto-commit)") {
			return hash, rows[i-1][0]
		}
	}
	t.Fatalf("no auto-commit containing %q found in dolt_log", messageSubstr)
	return "", ""
}

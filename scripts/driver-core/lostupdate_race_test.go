package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/types"
)

// TestDriverCore_LostUpdateRace is be-lo0sd: two disconnected clones of one
// corpus snapshot, each mutated independently with no shared state, are
// reconciled, and the result is checked against the replay-with-oracle
// harness's own tooling (be-hs42e.5.2's oracle-query, be-3j80a).
//
// Reconciliation routes through a shared, git-backed Dolt remote
// (git+file://, refs/dolt/data) -- the same mechanism cmd/bd/dolt.go's `bd
// dolt push`/`bd dolt pull` use in production, and the one this repo's own
// sync-concepts summary names ("sync uses refs/dolt/data on your git
// remote"). A direct file:// remote pointed at a peer's own .dolt/noms is
// not viable here: Dolt's remote-fetch path (LoadDoltDBWithParams) never
// sets dbfactory.ChunkJournalParam, so it cannot open a chunk-journal-format
// directory as a remote source at all (only a store's own self-open does),
// and every embedded store in this codebase runs in chunk-journal format.
//
// Two subtests isolate the two things this scenario measures:
//
//   - positive control: both clones edit the SAME field on the SAME issue
//     with no shared state -- a textbook lost-update race. Dolt's merge
//     must surface this as a real, non-empty conflict (storage.Conflict,
//     returned as data with a nil error -- see EmbeddedDoltStore.PullFrom)
//     and abort rather than silently pick a winner. The conflict count is
//     logged as a non-blocking METRIC_SAMPLE, never a scenario failure.
//   - negative control: the two clones edit two different issues, so no
//     row or key overlaps anywhere. This must reconcile with zero
//     conflicts, and the merged result -- queried back out through
//     oracle-query, exactly as the replay-with-oracle harness itself reads
//     history -- must match each clone's own oracle value field-for-field,
//     including attribution (issue_versions.change_actor).
func TestDriverCore_LostUpdateRace(t *testing.T) {
	requireDolt(t)
	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	toolsDir := t.TempDir()
	corpusAcquireBin := filepath.Join(toolsDir, "corpus-acquire")
	if err := goBuild(ctx, repoRoot, "./scripts/corpus-acquire", corpusAcquireBin); err != nil {
		t.Fatalf("building corpus-acquire: %v", err)
	}
	oracleQueryBin := filepath.Join(toolsDir, "oracle-query")
	if err := goBuild(ctx, repoRoot, "./scripts/oracle-query", oracleQueryBin); err != nil {
		t.Fatalf("building oracle-query: %v", err)
	}

	t.Run("concurrent_same_field_edit_is_a_detected_conflict_not_a_silent_lost_update", func(t *testing.T) {
		f := setupLostUpdateFixture(t, ctx, corpusAcquireBin)
		defer f.closeAll()

		if err := f.cloneA.UpdateIssue(ctx, f.sharedIssueID, map[string]interface{}{"title": "A's title"}, "actor-a"); err != nil {
			t.Fatalf("clone A update: %v", err)
		}
		// UpdateIssue only commits the SQL transaction against the working set
		// (EmbeddedDoltStore.withConn's "commit" is a database/sql tx.Commit,
		// not CALL DOLT_COMMIT -- see embeddeddolt/store.go's withConn doc and
		// ImportJSONLData's "Does NOT issue DOLT_COMMIT" comment). PushTo only
		// ever pushes real Dolt commits, so each clone's edit needs an
		// explicit Commit before push, exactly like setupLostUpdateFixture's
		// seed store already does after SetConfig.
		if err := f.cloneA.Commit(ctx, "A: update title"); err != nil {
			t.Fatalf("clone A commit: %v", err)
		}
		if err := f.cloneB.UpdateIssue(ctx, f.sharedIssueID, map[string]interface{}{"title": "B's title"}, "actor-b"); err != nil {
			t.Fatalf("clone B update: %v", err)
		}
		if err := f.cloneB.Commit(ctx, "B: update title"); err != nil {
			t.Fatalf("clone B commit: %v", err)
		}

		if err := f.cloneA.AddRemote(ctx, "shared", f.remoteURL); err != nil {
			t.Fatalf("clone A AddRemote: %v", err)
		}
		if err := f.cloneA.PushTo(ctx, "shared"); err != nil {
			t.Fatalf("clone A PushTo shared: %v", err)
		}
		if err := f.cloneB.AddRemote(ctx, "shared", f.remoteURL); err != nil {
			t.Fatalf("clone B AddRemote: %v", err)
		}
		conflicts, err := f.cloneB.PullFrom(ctx, "shared")
		if err != nil {
			t.Fatalf("clone B PullFrom shared: %v (a lost-update race must be reported as data -- a non-empty conflicts slice with a nil error -- never as a Go error)", err)
		}

		// METRIC_SAMPLE (non-blocking): fleet lost-update race count.
		// Recorded for observability; never fails the scenario on its own.
		//
		// len(conflicts) is not echoed from PullFrom's return by convention
		// alone, nor hard-coded: PullFrom (embeddeddolt/version_control.go)
		// wraps versioncontrolops.Pull, which on a real, unresolvable Dolt
		// merge conflict returns *MergeConflictsError{Conflicts: ...} built by
		// GetConflicts (versioncontrolops/version_control.go) -- a live query
		// against the reconciled Dolt state's own dolt_conflicts system view.
		// Each storage.Conflict.Count is "how many rows of the table are
		// conflicted, as reported by dolt_conflicts.num_conflicts" (see
		// storage.Conflict's doc comment, versioned.go). So this count is
		// read back out of Dolt's own post-merge history/state, the same
		// mechanism `dolt conflicts cat`/`bd vc conflicts` would report from.
		t.Logf("METRIC_SAMPLE lost_update_race_conflict_count=%d", len(conflicts))

		if len(conflicts) == 0 {
			t.Fatalf("clone A and clone B both edited %s's title independently with no shared state, but reconciling produced zero conflicts -- a same-field lost-update race must be surfaced, never silently merged", f.sharedIssueID)
		}
		var sawIssuesConflict bool
		for _, c := range conflicts {
			t.Logf("conflict: table=%s count=%d", c.Field, c.Count)
			if c.Field == "issues" {
				sawIssuesConflict = true
			}
		}
		if !sawIssuesConflict {
			t.Fatalf("conflicts=%+v: expected an `issues`-table conflict from the concurrent title edit, got none", conflicts)
		}

		// A conflicted pull aborts the merge (bd-578h9.15's settle machinery
		// rolls back what it can't auto-resolve before returning) -- so
		// clone B's own pre-pull state must survive completely intact: its
		// own edit is neither lost nor silently overwritten by A's.
		headB, err := headCommitOf(ctx, f.cloneBDir)
		if err != nil {
			t.Fatalf("headCommitOf(clone B, post-abort): %v", err)
		}
		got, err := QueryOracleSubprocess(ctx, oracleQueryBin, f.cloneBDir, headB, f.sharedIssueID)
		if err != nil {
			t.Fatalf("AS-OF query for %s at clone B's post-abort head %s: %v", f.sharedIssueID, headB, err)
		}
		if got == nil {
			t.Fatalf("AS-OF query for %s at clone B's post-abort head %s returned no row", f.sharedIssueID, headB)
		}
		if got["title"] != "B's title" {
			t.Errorf("clone B's own row after an aborted conflicted pull: title = %q, want %q (the aborted merge must never overwrite the puller's own state)", got["title"], "B's title")
		}
	})

	t.Run("concurrent_edits_to_different_issues_reconcile_cleanly_and_match_the_oracle", func(t *testing.T) {
		f := setupLostUpdateFixture(t, ctx, corpusAcquireBin)
		defer f.closeAll()

		issueA := &types.Issue{ID: "a-only", Title: "A's own issue", IssueType: types.TypeTask, Status: types.StatusOpen}
		if err := f.cloneA.CreateIssue(ctx, issueA, "actor-a"); err != nil {
			t.Fatalf("clone A create a-only: %v", err)
		}
		// CreateIssue only commits the SQL transaction against the working set,
		// never CALL DOLT_COMMIT -- see the matching comment on subtest 1's
		// UpdateIssue call. PushTo pushes committed history only, so without
		// this, a-only would never leave clone A's working set.
		if err := f.cloneA.Commit(ctx, "A: create a-only"); err != nil {
			t.Fatalf("clone A commit: %v", err)
		}
		issueB := &types.Issue{ID: "b-only", Title: "B's own issue", IssueType: types.TypeTask, Status: types.StatusOpen}
		if err := f.cloneB.CreateIssue(ctx, issueB, "actor-b"); err != nil {
			t.Fatalf("clone B create b-only: %v", err)
		}
		if err := f.cloneB.Commit(ctx, "B: create b-only"); err != nil {
			t.Fatalf("clone B commit: %v", err)
		}

		if err := f.cloneA.AddRemote(ctx, "shared", f.remoteURL); err != nil {
			t.Fatalf("clone A AddRemote: %v", err)
		}
		if err := f.cloneA.PushTo(ctx, "shared"); err != nil {
			t.Fatalf("clone A PushTo shared: %v", err)
		}
		if err := f.cloneB.AddRemote(ctx, "shared", f.remoteURL); err != nil {
			t.Fatalf("clone B AddRemote: %v", err)
		}
		conflicts, err := f.cloneB.PullFrom(ctx, "shared")
		if err != nil {
			t.Fatalf("clone B PullFrom shared: %v", err)
		}
		if len(conflicts) != 0 {
			t.Fatalf("clone A and clone B edited two different issues with no key overlap, but reconciling produced conflicts=%+v, want none", conflicts)
		}

		headB, err := headCommitOf(ctx, f.cloneBDir)
		if err != nil {
			t.Fatalf("headCommitOf(clone B, post-merge): %v", err)
		}

		for _, want := range []struct {
			issue *types.Issue
			actor string
		}{
			{issueA, "actor-a"},
			{issueB, "actor-b"},
		} {
			got, err := QueryOracleSubprocess(ctx, oracleQueryBin, f.cloneBDir, headB, want.issue.ID)
			if err != nil {
				t.Fatalf("AS-OF query for %s at reconciled head %s: %v", want.issue.ID, headB, err)
			}
			if got == nil {
				t.Fatalf("AS-OF query for %s at reconciled head %s returned no row -- the merge must not lose either side's issue", want.issue.ID, headB)
			}
			if got["title"] != want.issue.Title {
				t.Errorf("reconciled clone: issue %s title = %q, want %q (matching the oracle -- the clone that originated it)", want.issue.ID, got["title"], want.issue.Title)
			}

			header, rows, err := doltQuery(ctx, f.cloneBDir, "SELECT change_actor FROM issue_versions WHERE issue_id = '"+want.issue.ID+"' ORDER BY revision")
			if err != nil {
				t.Fatalf("querying issue_versions for %s: %v", want.issue.ID, err)
			}
			if len(rows) == 0 {
				t.Fatalf("issue_versions has no rows for %s after reconciling -- the merge must not lose either side's version history", want.issue.ID)
			}
			row := rowMap(header, rows[0])
			if row["change_actor"] != want.actor {
				t.Errorf("reconciled clone: issue_versions for %s change_actor = %q, want %q (attribution must survive the merge)", want.issue.ID, row["change_actor"], want.actor)
			}
		}
	})
}

// lostUpdateFixture is a pair of disconnected clones of one seeded corpus
// snapshot, acquired via corpus-acquire (never git clone, never dolt clone
// file:// against a chunk-journal store -- see be-hs42e.5.1), plus a shared
// bare git remote ready for reconciliation.
type lostUpdateFixture struct {
	cloneA, cloneB       *embeddeddolt.EmbeddedDoltStore
	cloneADir, cloneBDir string
	sharedIssueID        string
	remoteURL            string
}

func (f *lostUpdateFixture) closeAll() {
	_ = f.cloneA.Close()
	_ = f.cloneB.Close()
}

// setupLostUpdateFixture seeds one small corpus with a single issue, takes
// two independent filesystem-level clones of it via the corpus-acquire
// binary, and seeds a shared bare git repo ready for the caller to reconcile
// through. Each caller gets its own fixture so one subtest's mutations can
// never contaminate another's.
//
// embeddeddolt.Open(ctx, beadsDir, database, branch) always resolves the
// real Dolt directory to beadsDir/embeddeddolt/database/.dolt (see
// newStore, store.go) -- never beadsDir/.dolt directly. corpus-acquire's
// clone, by contrast, is deliberately flattened: AcquireCorpus copies
// straight to destDir/.dolt, with neither an "embeddeddolt" wrapper nor a
// database-named subdirectory, because its own consumers (oracle-query,
// this file's doltQuery/headCommitOf helpers) read a clone directory
// through the plain dolt CLI, which imposes no such nesting. So each clone
// here is acquired directly to <beadsDir>/embeddeddolt/<database> -- the
// exact path Open() will independently derive from that same beadsDir --
// which satisfies both conventions from the one copy, with no reshuffling
// and no second copy.
func setupLostUpdateFixture(t *testing.T, ctx context.Context, corpusAcquireBin string) *lostUpdateFixture {
	t.Helper()
	root := t.TempDir()
	const dbName = "lostupdate"

	seedBeadsDir := filepath.Join(root, "seed", ".beads")
	seed, err := embeddeddolt.Open(ctx, seedBeadsDir, dbName, "main")
	if err != nil {
		t.Fatalf("opening seed store: %v", err)
	}
	// A freshly opened store has no issue_prefix config yet, so every
	// mutation refuses with "database not initialized" -- mirrors
	// newPristineEmbeddedDoltFixture's own init dance (test_fixture_test.go)
	// for the same reason: there is no bd CLI in this scenario to run `bd
	// init --prefix` for us.
	if err := seed.SetConfig(ctx, "issue_prefix", dbName); err != nil {
		t.Fatalf("configuring seed store issue_prefix: %v", err)
	}
	if err := seed.Commit(ctx, "bd init"); err != nil {
		t.Fatalf("committing seed store init: %v", err)
	}
	seed.SetVersionedHistoryEnabled(true)
	const issueID = "shared-1"
	if err := seed.CreateIssue(ctx, &types.Issue{ID: issueID, Title: "seed", IssueType: types.TypeTask, Status: types.StatusOpen}, "seed-actor"); err != nil {
		t.Fatalf("seeding issue: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("closing seed store: %v", err)
	}
	sourceDataDir := filepath.Join(seedBeadsDir, "embeddeddolt")

	cloneABeadsDir := filepath.Join(root, "clone-a-beads")
	cloneADir := filepath.Join(cloneABeadsDir, "embeddeddolt", dbName)
	runCorpusAcquire(t, corpusAcquireBin, sourceDataDir, dbName, cloneADir)

	cloneBBeadsDir := filepath.Join(root, "clone-b-beads")
	cloneBDir := filepath.Join(cloneBBeadsDir, "embeddeddolt", dbName)
	runCorpusAcquire(t, corpusAcquireBin, sourceDataDir, dbName, cloneBDir)

	cloneA, err := embeddeddolt.Open(ctx, cloneABeadsDir, dbName, "main")
	if err != nil {
		t.Fatalf("opening clone A: %v", err)
	}
	cloneA.SetVersionedHistoryEnabled(true)
	cloneB, err := embeddeddolt.Open(ctx, cloneBBeadsDir, dbName, "main")
	if err != nil {
		_ = cloneA.Close()
		t.Fatalf("opening clone B: %v", err)
	}
	cloneB.SetVersionedHistoryEnabled(true)

	sharedGitDir := filepath.Join(root, "shared.git")
	initBareGitRepoWithInitialCommit(t, sharedGitDir)

	return &lostUpdateFixture{
		cloneA: cloneA, cloneB: cloneB,
		cloneADir: cloneADir, cloneBDir: cloneBDir,
		sharedIssueID: issueID,
		remoteURL:     "git+file://" + sharedGitDir,
	}
}

// runCorpusAcquire invokes the corpus-acquire binary to take a filesystem-
// level clone of dataDir/db into dest, failing the test on any error.
func runCorpusAcquire(t *testing.T, bin, dataDir, db, dest string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, "-data-dir", dataDir, "-db", db, "-dest", dest)
	cmd.Env = sanitizedEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("corpus-acquire -data-dir=%s -db=%s -dest=%s: %v\n%s", dataDir, db, dest, err, out)
	}
}

// initBareGitRepoWithInitialCommit creates a bare git repo at dir and seeds
// it with one empty initial commit on refs/heads/main. Dolt's git-backed
// remote (git+file://) requires the target to already have a branch before
// the first push: pushing to a bare repo with zero commits fails with "git
// remote has no branches ... initialize the repository with an initial
// branch/commit first" (empirically confirmed while validating this
// scenario's reconciliation mechanism).
func initBareGitRepoWithInitialCommit(t *testing.T, dir string) {
	t.Helper()
	runGitPlumbing(t, "", "init", "--bare", "-q", dir)
	tree := strings.TrimSpace(runGitPlumbing(t, dir, "hash-object", "-t", "tree", os.DevNull))
	commit := strings.TrimSpace(runGitPlumbing(t, dir, "commit-tree", tree, "-m", "init"))
	runGitPlumbing(t, dir, "update-ref", "refs/heads/main", commit)
	runGitPlumbing(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
}

// runGitPlumbing runs git with dir as its working directory (via -C; a bare
// repo needs no separate working tree for plumbing commands) and a
// sanitized child environment, failing the test on any error. dir == ""
// runs git with no -C, for "git init --bare <path>" itself.
func runGitPlumbing(t *testing.T, dir string, args ...string) string {
	t.Helper()
	var cmd *exec.Cmd
	if dir == "" {
		cmd = exec.CommandContext(context.Background(), "git", args...)
	} else {
		cmd = exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	}
	cmd.Env = sanitizedEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (dir=%s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

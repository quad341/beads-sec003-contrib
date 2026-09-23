//go:build cgo

package embeddeddolt_test

import (
	"fmt"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/types"
)

// migration0049DDL replays migration 0049's exact column widening: the four
// issues text columns move from TEXT to LONGTEXT NOT NULL. See
// internal/storage/schema/cli_migrations.go (cliMigration0049LongtextLargeContentColumns).
const migration0049DDL = "ALTER TABLE issues " +
	"MODIFY COLUMN description LONGTEXT NOT NULL, " +
	"MODIFY COLUMN design LONGTEXT NOT NULL, " +
	"MODIFY COLUMN acceptance_criteria LONGTEXT NOT NULL, " +
	"MODIFY COLUMN notes LONGTEXT NOT NULL"

// TestHistory_NullTextColumns reproduces GH#4867: dolt_history_issues
// projects every historical row against the CURRENT branch-head schema. A
// row committed while the issues text columns were still TEXT (pre-0049)
// type-mismatches the post-0049 LONGTEXT column definition when Dolt
// re-projects it, which surfaces as NULL rather than the original value.
// This is real migration behavior, not a hand-written NULL row: schema
// widening never mutates existing row bytes, only the type Dolt uses to
// project them.
func TestHistory_NullTextColumns(t *testing.T) {
	skipUnlessEmbeddedDolt(t)

	te := newTestEnv(t, "nh")
	ctx := t.Context()

	// (a) Simulate the pre-0049 schema: TEXT columns, not yet migrated.
	for _, col := range []string{"description", "design", "acceptance_criteria", "notes"} {
		te.exec(t, ctx, "ALTER TABLE issues MODIFY COLUMN `"+col+"` TEXT NOT NULL")
	}
	if err := te.store.Commit(ctx, "narrow issues text columns to TEXT (pre-0049 schema)"); err != nil {
		t.Fatalf("Commit (TEXT schema): %v", err)
	}

	// (b) Commit an issue under the pre-0049 TEXT schema. This becomes the
	// OLDER history entry.
	issue := &types.Issue{
		ID:                 "nh-null1",
		Title:              "Null history test",
		Description:        "original description",
		Design:             "original design",
		AcceptanceCriteria: "original AC",
		Notes:              "original notes",
		Status:             types.StatusOpen,
		Priority:           2,
		IssueType:          types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, issue, "tester"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := te.store.Commit(ctx, "initial commit under TEXT schema"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// (c) Replay migration 0049's exact DDL, widening to LONGTEXT. The row
	// data is untouched; only the branch-head column type changes. This
	// becomes the NEWEST history entry.
	te.exec(t, ctx, migration0049DDL)
	if err := te.store.Commit(ctx, "replay migration 0049 (TEXT -> LONGTEXT)"); err != nil {
		t.Fatalf("Commit (migration 0049): %v", err)
	}

	history, err := te.store.History(ctx, issue.ID)
	if err != nil {
		t.Fatalf("History() failed across a TEXT -> LONGTEXT migration: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected at least 2 history entries, got %d", len(history))
	}

	// Newest entry (post-migration commit): schema matches branch head, so
	// the real values project through untouched.
	newest := history[0].Issue
	if newest.Description != issue.Description {
		t.Errorf("expected newest description %q, got %q", issue.Description, newest.Description)
	}
	if newest.Design != issue.Design {
		t.Errorf("expected newest design %q, got %q", issue.Design, newest.Design)
	}
	if newest.AcceptanceCriteria != issue.AcceptanceCriteria {
		t.Errorf("expected newest acceptance_criteria %q, got %q", issue.AcceptanceCriteria, newest.AcceptanceCriteria)
	}
	if newest.Notes != issue.Notes {
		t.Errorf("expected newest notes %q, got %q", issue.Notes, newest.Notes)
	}

	// Older entry (pre-migration commit, TEXT-era): re-projected against the
	// current LONGTEXT schema, the type mismatch surfaces as NULL, which the
	// COALESCE in the scan turns into "".
	older := history[1].Issue
	if older.Description != "" {
		t.Errorf("expected pre-migration description to coalesce to \"\", got %q", older.Description)
	}
	if older.Design != "" {
		t.Errorf("expected pre-migration design to coalesce to \"\", got %q", older.Design)
	}
	if older.AcceptanceCriteria != "" {
		t.Errorf("expected pre-migration acceptance_criteria to coalesce to \"\", got %q", older.AcceptanceCriteria)
	}
	if older.Notes != "" {
		t.Errorf("expected pre-migration notes to coalesce to \"\", got %q", older.Notes)
	}
}

// TestHistory_UnrelatedCommitsDoNotBleedThrough reproduces be-qks1k: dolt_history_issues
// walks every commit reachable from HEAD and emits a row for any PK that
// existed in that commit's snapshot, regardless of whether that commit
// touched the PK's table at all. A commit that only edits an unrelated
// issue's row still produces a ghost row for this issue, carrying its prior
// value forward unchanged. dolt_diff_issues, by contrast, only emits a row
// for a PK when that specific commit actually changed its data.
func TestHistory_UnrelatedCommitsDoNotBleedThrough(t *testing.T) {
	skipUnlessEmbeddedDolt(t)

	te := newTestEnv(t, "bt")
	ctx := t.Context()

	target := &types.Issue{
		ID:        "bt-target1",
		Title:     "target v0",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, target, "tester"); err != nil {
		t.Fatalf("CreateIssue(target): %v", err)
	}
	if err := te.store.Commit(ctx, "create target issue"); err != nil {
		t.Fatalf("Commit (create target): %v", err)
	}

	other := &types.Issue{
		ID:        "bt-other1",
		Title:     "other v0",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, other, "tester"); err != nil {
		t.Fatalf("CreateIssue(other): %v", err)
	}
	if err := te.store.Commit(ctx, "create other issue"); err != nil {
		t.Fatalf("Commit (create other): %v", err)
	}

	if err := te.store.UpdateIssue(ctx, target.ID, map[string]interface{}{"title": "target v1"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue(target): %v", err)
	}
	if err := te.store.Commit(ctx, "edit target issue"); err != nil {
		t.Fatalf("Commit (edit target): %v", err)
	}

	// 3 more commits that touch ONLY the unrelated issue. Under the current
	// dolt_history_issues-based query, each of these still produces a ghost
	// row for target (its value carried forward unchanged) because target
	// existed in every one of these commits' snapshots.
	for i := 0; i < 3; i++ {
		if err := te.store.UpdateIssue(ctx, other.ID, map[string]interface{}{"title": fmt.Sprintf("other v%d", i+1)}, "tester"); err != nil {
			t.Fatalf("UpdateIssue(other #%d): %v", i, err)
		}
		if err := te.store.Commit(ctx, fmt.Sprintf("unrelated edit %d", i+1)); err != nil {
			t.Fatalf("Commit (unrelated %d): %v", i, err)
		}
	}

	history, err := te.store.History(ctx, target.ID)
	if err != nil {
		t.Fatalf("History(target): %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected exactly 2 history entries for target (create + 1 real edit), got %d — unrelated commits to a different issue must not bleed through", len(history))
	}
	if history[0].Issue.Title != "target v1" {
		t.Errorf("expected newest entry title %q, got %q", "target v1", history[0].Issue.Title)
	}
	if history[1].Issue.Title != "target v0" {
		t.Errorf("expected oldest entry title %q, got %q", "target v0", history[1].Issue.Title)
	}
}

// TestHistory_MultipleEditsSurviveMax1RowOptimization defeats Dolt's max1Row
// query-planner optimization, which can incorrectly assume a bare `WHERE
// id=?` predicate on a history/diff system table returns at most one row per
// PK. With 3 real edits (4 real-change commits total, each followed by an
// unrelated-only commit) all 4 must still be returned — none silently
// truncated by the planner and none of the 3 unrelated commits bleeding
// through.
func TestHistory_MultipleEditsSurviveMax1RowOptimization(t *testing.T) {
	skipUnlessEmbeddedDolt(t)

	te := newTestEnv(t, "mr")
	ctx := t.Context()

	target := &types.Issue{
		ID:        "mr-target1",
		Title:     "v0",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, target, "tester"); err != nil {
		t.Fatalf("CreateIssue(target): %v", err)
	}
	if err := te.store.Commit(ctx, "create target issue"); err != nil {
		t.Fatalf("Commit (create target): %v", err)
	}

	other := &types.Issue{
		ID:        "mr-other1",
		Title:     "other v0",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, other, "tester"); err != nil {
		t.Fatalf("CreateIssue(other): %v", err)
	}
	if err := te.store.Commit(ctx, "create other issue"); err != nil {
		t.Fatalf("Commit (create other): %v", err)
	}

	for i := 1; i <= 3; i++ {
		newTitle := fmt.Sprintf("v%d", i)
		if err := te.store.UpdateIssue(ctx, target.ID, map[string]interface{}{"title": newTitle}, "tester"); err != nil {
			t.Fatalf("UpdateIssue(target -> %s): %v", newTitle, err)
		}
		if err := te.store.Commit(ctx, fmt.Sprintf("edit target to %s", newTitle)); err != nil {
			t.Fatalf("Commit (edit %d): %v", i, err)
		}

		if err := te.store.UpdateIssue(ctx, other.ID, map[string]interface{}{"title": fmt.Sprintf("other v%d", i)}, "tester"); err != nil {
			t.Fatalf("UpdateIssue(other #%d): %v", i, err)
		}
		if err := te.store.Commit(ctx, fmt.Sprintf("unrelated edit %d", i)); err != nil {
			t.Fatalf("Commit (unrelated %d): %v", i, err)
		}
	}

	history, err := te.store.History(ctx, target.ID)
	if err != nil {
		t.Fatalf("History(target): %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected exactly 4 history entries (create + 3 edits), got %d — either max1Row truncated real edits or unrelated commits bled through", len(history))
	}

	wantTitles := []string{"v3", "v2", "v1", "v0"}
	for i, want := range wantTitles {
		if history[i].Issue.Title != want {
			t.Errorf("history[%d]: expected title %q, got %q", i, want, history[i].Issue.Title)
		}
	}
}

// TestHistory_DeletedIssueShowsFinalEntry verifies that deleting an issue
// still surfaces a final history entry attributed to the deletion commit
// itself. dolt_history_issues cannot do this: a deleted row is absent from
// the deletion commit's own snapshot, so the walk skips straight past it to
// the last commit where the row still existed (the create commit here) —
// the deletion never gets its own entry. dolt_diff_issues, which diffs
// adjacent commits rather than walking snapshots, sees the row present on
// one side and gone on the other and emits a real "removed" diff row for
// the deletion commit.
func TestHistory_DeletedIssueShowsFinalEntry(t *testing.T) {
	skipUnlessEmbeddedDolt(t)

	te := newTestEnv(t, "del")
	ctx := t.Context()

	issue := &types.Issue{
		ID:        "del-target1",
		Title:     "to be deleted",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, issue, "tester"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := te.store.Commit(ctx, "create issue"); err != nil {
		t.Fatalf("Commit (create): %v", err)
	}

	if err := te.store.DeleteIssue(ctx, issue.ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	if err := te.store.Commit(ctx, "delete issue"); err != nil {
		t.Fatalf("Commit (delete): %v", err)
	}
	deleteSHA, err := te.store.GetCurrentCommit(ctx)
	if err != nil {
		t.Fatalf("GetCurrentCommit: %v", err)
	}

	history, err := te.store.History(ctx, issue.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) == 0 {
		t.Fatalf("expected at least 1 history entry for a deleted issue, got 0")
	}
	if history[0].CommitHash != deleteSHA {
		t.Errorf("expected newest history entry to be the deletion commit %q, got %q", deleteSHA, history[0].CommitHash)
	}
	if history[0].Issue == nil || history[0].Issue.ID != issue.ID {
		t.Errorf("expected deletion entry to still identify issue %q", issue.ID)
	}
}

// TestHistory_BranchMergeAttributesEditOnce is exploratory: at design time,
// Dolt's merge-commit diff attribution for a clean (non-conflicting)
// side-branch edit merged back into main was not empirically verified. A
// side-branch edit should be attributed exactly once — not once for the
// real edit commit and again for a ghost row on the merge commit itself. If
// this fails with a count other than 1, that is a real finding to document
// during the GREEN step, not a test bug to silence.
//
// The branch/checkout/merge sequence needs its own pinned connection (Dolt
// checkout state is session-scoped), opened directly here instead of via
// the shared openSettleConn helper (pull_settle_test.go): that helper's
// engine stays open until t.Cleanup, but EmbeddedDoltStore.withConn opens a
// competing OpenSQL engine against the same on-disk directory on every
// te.store call, and OpenSQL's retry backoff never gives up
// (open.go: bo.MaxElapsedTime = 0, wait until ctx cancellation) — so calling
// te.store.History while the settle engine is still open deadlocks both
// sides until the test binary's own 10-minute timeout fires. This is why
// every other test in pull_settle_test.go asserts via raw SQL on the pinned
// conn instead of calling back into te.store; here the pinned engine is
// closed explicitly before te.store.History runs, so the real production
// path still gets exercised.
func TestHistory_BranchMergeAttributesEditOnce(t *testing.T) {
	skipUnlessEmbeddedDolt(t)

	te := newTestEnv(t, "bm")
	ctx := t.Context()

	issue := &types.Issue{
		ID:        "bm-merge1",
		Title:     "original title",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, issue, "tester"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := te.store.Commit(ctx, "create issue on main"); err != nil {
		t.Fatalf("Commit (create): %v", err)
	}

	// A second issue, edited on main after the branch point (below) so main
	// diverges from the branch point too. Without this, DOLT_MERGE
	// fast-forwards instead of creating a real merge commit — main's HEAD
	// would just move to the side branch's own edit commit, leaving nothing
	// for a merge-commit ghost row to bleed through from, so the assertion
	// below would pass trivially under both the old and new implementations.
	other := &types.Issue{
		ID:        "bm-other1",
		Title:     "other v0",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := te.store.CreateIssue(ctx, other, "tester"); err != nil {
		t.Fatalf("CreateIssue(other): %v", err)
	}
	if err := te.store.Commit(ctx, "create other issue on main"); err != nil {
		t.Fatalf("Commit (create other): %v", err)
	}

	db, cleanup, err := embeddeddolt.OpenSQL(ctx, te.dataDir, te.database, "main")
	if err != nil {
		t.Fatalf("OpenSQL: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("pin connection: %v", err)
	}
	settleClosed := false
	closeSettle := func() {
		if settleClosed {
			return
		}
		settleClosed = true
		if cErr := conn.Close(); cErr != nil {
			t.Errorf("close pinned connection: %v", cErr)
		}
		if cErr := cleanup(); cErr != nil {
			t.Errorf("cleanup settle engine: %v", cErr)
		}
	}
	defer closeSettle()

	const sideBranch = "bm-merge1-side"
	if _, err := conn.ExecContext(ctx, "CALL DOLT_BRANCH(?, 'HEAD')", sideBranch); err != nil {
		t.Fatalf("DOLT_BRANCH: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "CALL DOLT_CHECKOUT(?)", sideBranch); err != nil {
		t.Fatalf("DOLT_CHECKOUT(%s): %v", sideBranch, err)
	}

	const newTitle = "edited on side branch"
	if _, err := conn.ExecContext(ctx, "UPDATE issues SET title = ? WHERE id = ?", newTitle, issue.ID); err != nil {
		t.Fatalf("UPDATE (side branch): %v", err)
	}
	if _, err := conn.ExecContext(ctx, "CALL DOLT_COMMIT('-Am', 'edit title on side branch')"); err != nil {
		t.Fatalf("DOLT_COMMIT (side branch): %v", err)
	}

	if _, err := conn.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')"); err != nil {
		t.Fatalf("DOLT_CHECKOUT(main): %v", err)
	}

	// Diverge main from the branch point (see the comment on `other` above).
	if _, err := conn.ExecContext(ctx, "UPDATE issues SET title = ? WHERE id = ?", "other v1", other.ID); err != nil {
		t.Fatalf("UPDATE (main): %v", err)
	}
	if _, err := conn.ExecContext(ctx, "CALL DOLT_COMMIT('-Am', 'unrelated edit on main')"); err != nil {
		t.Fatalf("DOLT_COMMIT (main): %v", err)
	}

	if _, err := conn.ExecContext(ctx, "CALL DOLT_MERGE(?)", sideBranch); err != nil {
		t.Fatalf("DOLT_MERGE(%s): %v", sideBranch, err)
	}

	// Release the settle engine's lock on the on-disk directory before
	// te.store opens its own competing engine below.
	closeSettle()

	history, err := te.store.History(ctx, issue.ID)
	if err != nil {
		t.Fatalf("History() after branch merge: %v", err)
	}

	occurrences := 0
	for _, entry := range history {
		if entry.Issue.Title == newTitle {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Errorf("expected side-branch edit title %q to be attributed exactly once in history, got %d occurrences (history len=%d)", newTitle, occurrences, len(history))
	}
}

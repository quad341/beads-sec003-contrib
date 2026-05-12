//go:build integration_pg

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// renameTextFor builds an Issue carrying the new content payload that
// UpdateIssueID applies in the same UPDATE that changes the ID. Matches the
// shape cmd/bd/rename.go:80 passes in.
func renameTextFor(id, title string) *types.Issue {
	return &types.Issue{
		ID:                 id,
		Title:              title,
		Description:        "updated desc",
		Design:             "updated design",
		AcceptanceCriteria: "updated acceptance",
		Notes:              "updated notes",
	}
}

// countEvents counts rows in the named event table whose issue_id and
// event_type match. Used to verify the audit row count.
func countEvents(t *testing.T, store *PostgresStore, ctx context.Context, table, issueID, eventType string) int {
	t.Helper()
	var n int
	//nolint:gosec // table is a constant test-side string
	q := "SELECT COUNT(*) FROM " + table + " WHERE issue_id = $1 AND event_type = $2"
	if err := store.pool.QueryRow(ctx, q, issueID, eventType).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", table, err)
	}
	return n
}

// 1. Vanilla rename: row gone at oldID, present at newID, single 'renamed' event.
func TestUpdateIssueID_Vanilla(t *testing.T) {
	store, ctx := makeStore(t)
	oldID := makeIssue(t, store, ctx, "before")

	newID := oldID + "-renamed"
	if err := store.UpdateIssueID(ctx, oldID, newID, renameTextFor(newID, "after"), "tester"); err != nil {
		t.Fatalf("UpdateIssueID: %v", err)
	}

	if countRows(t, store, ctx, "issues", "id", oldID) != 0 {
		t.Errorf("oldID still present in issues")
	}
	if countRows(t, store, ctx, "issues", "id", newID) != 1 {
		t.Errorf("newID not present in issues")
	}
	if got := countEvents(t, store, ctx, "events", newID, "renamed"); got != 1 {
		t.Errorf("expected 1 renamed event, got %d", got)
	}

	// New title/description/notes payload landed.
	var gotTitle string
	if err := store.pool.QueryRow(ctx,
		"SELECT title FROM issues WHERE id = $1", newID,
	).Scan(&gotTitle); err != nil {
		t.Fatalf("read title: %v", err)
	}
	if gotTitle != "after" {
		t.Errorf("title not updated: %q", gotTitle)
	}
}

// 2. Full cross-reference rename: labels + comments + events + outbound + inbound deps.
func TestUpdateIssueID_AllReferencesMigrate(t *testing.T) {
	store, ctx := makeStore(t)
	subject := makeIssue(t, store, ctx, "subject")
	other := makeIssue(t, store, ctx, "other")
	thirdParty := makeIssue(t, store, ctx, "thirdparty")

	// Outbound: subject depends on other.
	makeBlocks(t, store, ctx, other, subject)
	// Inbound: thirdParty depends on subject.
	makeBlocks(t, store, ctx, subject, thirdParty)

	if err := store.AddLabel(ctx, subject, "bug", "tester"); err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if err := store.AddLabel(ctx, subject, "regression", "tester"); err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if _, err := store.AddIssueComment(ctx, subject, "tester", "first"); err != nil {
		t.Fatalf("AddIssueComment: %v", err)
	}

	newID := subject + "-r"
	if err := store.UpdateIssueID(ctx, subject, newID, renameTextFor(newID, "subject"), "tester"); err != nil {
		t.Fatalf("UpdateIssueID: %v", err)
	}

	// FK CASCADE: labels, comments, events, issue_id side of dependencies, snapshots.
	if got := countRows(t, store, ctx, "labels", "issue_id", newID); got != 2 {
		t.Errorf("labels not migrated: %d", got)
	}
	if got := countRows(t, store, ctx, "comments", "issue_id", newID); got != 1 {
		t.Errorf("comments not migrated: %d", got)
	}
	// Outbound (issue_id side): subject's row should now use newID.
	if got := countRows(t, store, ctx, "dependencies", "issue_id", newID); got != 1 {
		t.Errorf("outbound dep not migrated: %d", got)
	}
	// Manual update: dependencies.depends_on_id side (thirdParty → subject became newID).
	if got := countRows(t, store, ctx, "dependencies", "depends_on_id", newID); got != 1 {
		t.Errorf("inbound dep not migrated: %d", got)
	}

	// Nothing left at old ID anywhere.
	for _, table := range []string{"issues", "labels", "comments", "dependencies"} {
		col := "issue_id"
		if table == "issues" {
			col = "id"
		}
		if got := countRows(t, store, ctx, table, col, subject); got != 0 {
			t.Errorf("stale %s.%s rows for old ID: %d", table, col, got)
		}
	}
	if got := countRows(t, store, ctx, "dependencies", "depends_on_id", subject); got != 0 {
		t.Errorf("stale inbound dep at old ID: %d", got)
	}

	// Renamed event recorded exactly once.
	if got := countEvents(t, store, ctx, "events", newID, "renamed"); got != 1 {
		t.Errorf("expected 1 renamed event, got %d", got)
	}
}

// 3. Rename when target ID is already used → typed error, no writes.
func TestUpdateIssueID_TargetExists(t *testing.T) {
	store, ctx := makeStore(t)
	oldID := makeIssue(t, store, ctx, "old")
	taken := makeIssue(t, store, ctx, "taken")

	err := store.UpdateIssueID(ctx, oldID, taken, renameTextFor(taken, "old"), "tester")
	if err == nil {
		t.Fatal("expected error renaming onto occupied ID")
	}
	if !strings.Contains(err.Error(), "target ID already in use") {
		t.Errorf("unexpected error message: %v", err)
	}
	// Both originals still exist with their titles unchanged.
	for _, id := range []string{oldID, taken} {
		if countRows(t, store, ctx, "issues", "id", id) != 1 {
			t.Errorf("row missing for %s after failed rename", id)
		}
	}
	// No spurious renamed events recorded.
	if got := countEvents(t, store, ctx, "events", taken, "renamed"); got != 0 {
		t.Errorf("rollback left an event behind: %d", got)
	}
}

// 4. Rename a wisp → only wisp_* tables touched, wisp_events records the rename.
func TestUpdateIssueID_Wisp(t *testing.T) {
	store, ctx := makeStore(t)
	wispOld := "bd-wisp-old1"
	insertWispDirect(t, store, ctx, wispOld, "wisp before")

	// Seed wisp_* aux rows for the wisp.
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO wisp_labels (issue_id, label) VALUES ($1, $2)", wispOld, "triage"); err != nil {
		t.Fatalf("seed wisp_labels: %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO wisp_dependencies (issue_id, depends_on_id, type) VALUES ($1, $2, $3)",
		wispOld, "some-target", "blocks"); err != nil {
		t.Fatalf("seed wisp_dependencies issue_id: %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO wisp_dependencies (issue_id, depends_on_id, type) VALUES ($1, $2, $3)",
		"some-source", wispOld, "blocks"); err != nil {
		t.Fatalf("seed wisp_dependencies depends_on_id: %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO wisp_comments (issue_id, author, text) VALUES ($1, $2, $3)",
		wispOld, "tester", "test comment"); err != nil {
		t.Fatalf("seed wisp_comments: %v", err)
	}

	wispNew := "bd-wisp-new1"
	if err := store.UpdateIssueID(ctx, wispOld, wispNew, renameTextFor(wispNew, "wisp after"), "tester"); err != nil {
		t.Fatalf("UpdateIssueID(wisp): %v", err)
	}

	// Old ID gone everywhere.
	if got := countRows(t, store, ctx, "wisps", "id", wispOld); got != 0 {
		t.Errorf("wisp row at old ID survived: %d", got)
	}
	if got := countRows(t, store, ctx, "wisps", "id", wispNew); got != 1 {
		t.Errorf("wisp row at new ID missing: %d", got)
	}
	if got := countRows(t, store, ctx, "wisp_labels", "issue_id", wispNew); got != 1 {
		t.Errorf("wisp_labels not migrated: %d", got)
	}
	if got := countRows(t, store, ctx, "wisp_dependencies", "issue_id", wispNew); got != 1 {
		t.Errorf("wisp_dependencies (issue_id) not migrated: %d", got)
	}
	if got := countRows(t, store, ctx, "wisp_dependencies", "depends_on_id", wispNew); got != 1 {
		t.Errorf("wisp_dependencies (depends_on_id) not migrated: %d", got)
	}
	if got := countRows(t, store, ctx, "wisp_comments", "issue_id", wispNew); got != 1 {
		t.Errorf("wisp_comments not migrated: %d", got)
	}
	if got := countEvents(t, store, ctx, "wisp_events", wispNew, "renamed"); got != 1 {
		t.Errorf("expected 1 wisp 'renamed' event, got %d", got)
	}
	// Persistent tables untouched.
	if got := countEvents(t, store, ctx, "events", wispNew, "renamed"); got != 0 {
		t.Errorf("wisp rename leaked into events: %d", got)
	}
	if got := countRows(t, store, ctx, "issues", "id", wispNew); got != 0 {
		t.Errorf("wisp rename created an issues row: %d", got)
	}
}

// 5. Rename a hierarchical-ID parent whose child_counters.parent_id is set.
func TestUpdateIssueID_PreservesChildCounter(t *testing.T) {
	store, ctx := makeStore(t)
	parent := makeIssue(t, store, ctx, "parent")

	// Advance the child counter twice so it carries a non-trivial last_child value.
	first, err := store.GetNextChildID(ctx, parent)
	if err != nil {
		t.Fatalf("GetNextChildID: %v", err)
	}
	if !strings.HasPrefix(first, parent+".") {
		t.Errorf("unexpected first child id: %q", first)
	}
	if _, err := store.GetNextChildID(ctx, parent); err != nil {
		t.Fatalf("GetNextChildID(2): %v", err)
	}

	newID := parent + "-r"
	if err := store.UpdateIssueID(ctx, parent, newID, renameTextFor(newID, "parent"), "tester"); err != nil {
		t.Fatalf("UpdateIssueID: %v", err)
	}

	// Old counter row gone, new one present with last_child=2 preserved.
	if got := countRows(t, store, ctx, "child_counters", "parent_id", parent); got != 0 {
		t.Errorf("old counter row survived: %d", got)
	}
	var last int
	if err := store.pool.QueryRow(ctx,
		"SELECT last_child FROM child_counters WHERE parent_id = $1", newID,
	).Scan(&last); err != nil {
		t.Fatalf("read child_counters: %v", err)
	}
	if last != 2 {
		t.Errorf("counter not preserved: last_child=%d", last)
	}

	// Next allocation continues from the preserved counter.
	next, err := store.GetNextChildID(ctx, newID)
	if err != nil {
		t.Fatalf("GetNextChildID(post-rename): %v", err)
	}
	if !strings.HasSuffix(next, ".3") {
		t.Errorf("next child id did not advance: %q", next)
	}
}

// 6. newID == oldID → no-op, no error, no audit event.
func TestUpdateIssueID_NoOp(t *testing.T) {
	store, ctx := makeStore(t)
	id := makeIssue(t, store, ctx, "same")

	if err := store.UpdateIssueID(ctx, id, id, renameTextFor(id, "same"), "tester"); err != nil {
		t.Fatalf("no-op rename returned error: %v", err)
	}
	if got := countEvents(t, store, ctx, "events", id, "renamed"); got != 0 {
		t.Errorf("no-op rename recorded an event: %d", got)
	}
}

// 7. Migration 0004 idempotency: running migrations against an already-migrated DB is a no-op.
func TestMigration0004_Idempotent(t *testing.T) {
	store, ctx := makeStore(t)

	// Confirm v4 was applied during makeStore's openStore.
	var version int
	if err := store.pool.QueryRow(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM bd_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	if version < 4 {
		t.Fatalf("migration 0004 was not applied: max version = %d", version)
	}

	// Re-running should be a no-op (the loop skips migrations <= current).
	if err := store.runMigrations(ctx, false); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}

	// CASCADE-rename should still work after the second runMigrations pass.
	oldID := makeIssue(t, store, ctx, "after re-run")
	newID := oldID + "-r"
	if err := store.UpdateIssueID(ctx, oldID, newID, renameTextFor(newID, "after re-run"), "tester"); err != nil {
		t.Fatalf("rename after re-run: %v", err)
	}
}

// 8. Rename non-existent issue → typed error, no writes.
func TestUpdateIssueID_NotFound(t *testing.T) {
	store, ctx := makeStore(t)
	err := store.UpdateIssueID(ctx, "nope-1", "nope-2", renameTextFor("nope-2", "ghost"), "tester")
	if err == nil {
		t.Fatal("expected error renaming non-existent issue")
	}
	if !strings.Contains(err.Error(), "issue not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// 9. CASCADE coverage smoke: directly assert ON UPDATE CASCADE is in place for
// all six FKs after migration 0004. Catches future regressions where a
// migration redefines a FK without ON UPDATE CASCADE.
func TestMigration0004_FKsHaveOnUpdateCascade(t *testing.T) {
	store, ctx := makeStore(t)

	want := []struct {
		table      string
		constraint string
	}{
		{"dependencies", "fk_dep_issue"},
		{"labels", "fk_labels_issue"},
		{"comments", "fk_comments_issue"},
		{"events", "fk_events_issue"},
		{"issue_snapshots", "fk_snapshots_issue"},
		{"compaction_snapshots", "fk_comp_snap_issue"},
	}
	for _, w := range want {
		var action string
		if err := store.pool.QueryRow(ctx, `
			SELECT confupdtype::text
			FROM pg_constraint
			WHERE conname = $1
			  AND conrelid = $2::regclass
		`, w.constraint, w.table).Scan(&action); err != nil {
			t.Errorf("look up %s on %s: %v", w.constraint, w.table, err)
			continue
		}
		// confupdtype: 'c' = CASCADE, 'a' = NO ACTION (default).
		if action != "c" {
			t.Errorf("%s.%s confupdtype = %q, want 'c'", w.table, w.constraint, action)
		}
	}
}

// 10. Round-trip via a fresh transaction confirms the rename is visible to
// subsequent reads — guards against any caller forgetting to commit.
func TestUpdateIssueID_VisibleAfterCommit(t *testing.T) {
	store, ctx := makeStore(t)
	oldID := makeIssue(t, store, ctx, "visible")
	newID := oldID + "-r"

	if err := store.UpdateIssueID(ctx, oldID, newID, renameTextFor(newID, "visible after"), "tester"); err != nil {
		t.Fatalf("UpdateIssueID: %v", err)
	}

	got, err := store.GetIssue(ctx, newID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Title != "visible after" {
		t.Errorf("title not applied: %q", got.Title)
	}

	if _, err := store.GetIssue(ctx, oldID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound for oldID, got %v", err)
	}
}

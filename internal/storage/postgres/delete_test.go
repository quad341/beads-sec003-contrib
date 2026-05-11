//go:build integration_pg

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// makeStore boots a per-test PG store with issue_prefix set.
func makeStore(t *testing.T) (*PostgresStore, context.Context) {
	t.Helper()
	dsn := startPGFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SetConfig(ctx, "issue_prefix", "bd"); err != nil {
		t.Fatalf("set prefix: %v", err)
	}
	return store, ctx
}

// startPGFixture is a wrapper around startPG that returns just the DSN.
// startPG (integration_test.go) returns a (dsn, no-op-closer) pair from the
// shared testfixture helper.
func startPGFixture(t *testing.T) string {
	t.Helper()
	dsn, _ := startPG(t)
	return dsn
}

// makeIssue creates a persistent issue with the given title and returns its ID.
func makeIssue(t *testing.T, store *PostgresStore, ctx context.Context, title string) string {
	t.Helper()
	issue := &types.Issue{
		Title:     title,
		IssueType: types.TypeTask,
		Priority:  2,
		Status:    types.StatusOpen,
	}
	if err := store.CreateIssue(ctx, issue, "tester"); err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return issue.ID
}

// makeBlocks adds an issue blocks-edge from blocker to blocked: blocked
// depends on blocker (blocker must close first for blocked to be ready).
func makeBlocks(t *testing.T, store *PostgresStore, ctx context.Context, blocker, blocked string) {
	t.Helper()
	if err := store.AddDependency(ctx, &types.Dependency{
		IssueID:     blocked,
		DependsOnID: blocker,
		Type:        types.DepBlocks,
	}, "tester"); err != nil {
		t.Fatalf("add dep %s→%s: %v", blocker, blocked, err)
	}
}

// insertWispDirect bypasses the public API to insert a wisp row directly.
// Required because PostgresStore currently has no public CreateWisp method —
// wisps land via the migration path. For delete tests we just need a row in
// the wisps table whose ID we can pass to DeleteIssues.
func insertWispDirect(t *testing.T, store *PostgresStore, ctx context.Context, id, title string) {
	t.Helper()
	_, err := store.pool.Exec(ctx,
		"INSERT INTO wisps (id, title) VALUES ($1, $2)", id, title)
	if err != nil {
		t.Fatalf("insert wisp %s: %v", id, err)
	}
}

// countRows is a small helper used by the CASCADE-assertion test.
func countRows(t *testing.T, store *PostgresStore, ctx context.Context, table, whereCol, whereVal string) int {
	t.Helper()
	var n int
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", table, whereCol)
	if err := store.pool.QueryRow(ctx, q, whereVal).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// --- Tests ---

// 1. Empty ids → empty result, no error.
func TestDeleteIssues_EmptyIDs(t *testing.T) {
	store, ctx := makeStore(t)
	result, err := store.DeleteIssues(ctx, nil, false, false, false)
	if err != nil {
		t.Fatalf("delete empty: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.DeletedCount != 0 {
		t.Errorf("expected DeletedCount=0, got %d", result.DeletedCount)
	}
}

// 2. Single issue, no deps, cascade=false → DeletedCount=1.
func TestDeleteIssues_SingleNoDeps(t *testing.T) {
	store, ctx := makeStore(t)
	id := makeIssue(t, store, ctx, "Loner")

	result, err := store.DeleteIssues(ctx, []string{id}, false, false, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.DeletedCount != 1 {
		t.Errorf("expected DeletedCount=1, got %d", result.DeletedCount)
	}
	if _, err := store.GetIssue(ctx, id); err == nil {
		t.Errorf("expected %s gone, still readable", id)
	}
}

// 3. A blocks B, ids=[A], cascade=false, force=false → error, OrphanedIssues=[B].
// Verifies the row stays unchanged.
func TestDeleteIssues_OrphanCheckBlocks(t *testing.T) {
	store, ctx := makeStore(t)
	a := makeIssue(t, store, ctx, "A")
	b := makeIssue(t, store, ctx, "B")
	makeBlocks(t, store, ctx, a, b)

	result, err := store.DeleteIssues(ctx, []string{a}, false, false, false)
	if err == nil {
		t.Fatal("expected error reporting orphan, got nil")
	}
	if !strings.Contains(err.Error(), "dependents not in deletion set") {
		t.Errorf("expected orphan error message, got %v", err)
	}
	if result == nil || len(result.OrphanedIssues) != 1 || result.OrphanedIssues[0] != b {
		t.Errorf("expected OrphanedIssues=[%s], got %+v", b, result)
	}
	// Row must remain.
	if _, err := store.GetIssue(ctx, a); err != nil {
		t.Errorf("expected %s still readable, got %v", a, err)
	}
}

// 4. A blocks B, ids=[A], cascade=true → DeletedCount=2, both gone.
func TestDeleteIssues_CascadeDependents(t *testing.T) {
	store, ctx := makeStore(t)
	a := makeIssue(t, store, ctx, "A")
	b := makeIssue(t, store, ctx, "B")
	makeBlocks(t, store, ctx, a, b)

	result, err := store.DeleteIssues(ctx, []string{a}, true, false, false)
	if err != nil {
		t.Fatalf("delete cascade: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Errorf("expected DeletedCount=2, got %d", result.DeletedCount)
	}
	if _, err := store.GetIssue(ctx, a); err == nil {
		t.Errorf("expected %s gone", a)
	}
	if _, err := store.GetIssue(ctx, b); err == nil {
		t.Errorf("expected %s gone", b)
	}
}

// 5. A blocks B, ids=[A], force=true → DeletedCount=1, OrphanedIssues=[B],
// B remains, inbound edge from A→B gone.
func TestDeleteIssues_ForceLeavesOrphans(t *testing.T) {
	store, ctx := makeStore(t)
	a := makeIssue(t, store, ctx, "A")
	b := makeIssue(t, store, ctx, "B")
	makeBlocks(t, store, ctx, a, b)

	result, err := store.DeleteIssues(ctx, []string{a}, false, true, false)
	if err != nil {
		t.Fatalf("delete force: %v", err)
	}
	if result.DeletedCount != 1 {
		t.Errorf("expected DeletedCount=1, got %d", result.DeletedCount)
	}
	if len(result.OrphanedIssues) != 1 || result.OrphanedIssues[0] != b {
		t.Errorf("expected OrphanedIssues=[%s], got %+v", b, result.OrphanedIssues)
	}
	if _, err := store.GetIssue(ctx, a); err == nil {
		t.Errorf("expected %s gone", a)
	}
	if _, err := store.GetIssue(ctx, b); err != nil {
		t.Errorf("expected %s still readable, got %v", b, err)
	}
	if n := countRows(t, store, ctx, "dependencies", "issue_id", b); n != 0 {
		t.Errorf("expected no remaining dep edges for B, got %d", n)
	}
}

// 6. Mixed list (1 wisp + 1 regular) → both deleted.
func TestDeleteIssues_MixedWispRegular(t *testing.T) {
	store, ctx := makeStore(t)
	regular := makeIssue(t, store, ctx, "Regular")
	const wispID = "be-wisp-mixed-1"
	insertWispDirect(t, store, ctx, wispID, "Wisp")

	result, err := store.DeleteIssues(ctx, []string{regular, wispID}, false, false, false)
	if err != nil {
		t.Fatalf("delete mixed: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Errorf("expected DeletedCount=2, got %d", result.DeletedCount)
	}
	if n := countRows(t, store, ctx, "wisps", "id", wispID); n != 0 {
		t.Errorf("expected wisp gone, %d rows remain", n)
	}
	if n := countRows(t, store, ctx, "issues", "id", regular); n != 0 {
		t.Errorf("expected regular issue gone, %d rows remain", n)
	}
}

// 7. dryRun=true with cascade=true → counts populated, all rows still present.
func TestDeleteIssues_DryRunCascade(t *testing.T) {
	store, ctx := makeStore(t)
	a := makeIssue(t, store, ctx, "A")
	b := makeIssue(t, store, ctx, "B")
	makeBlocks(t, store, ctx, a, b)
	if err := store.AddLabel(ctx, a, "alpha", "tester"); err != nil {
		t.Fatalf("label A: %v", err)
	}

	result, err := store.DeleteIssues(ctx, []string{a}, true, false, true)
	if err != nil {
		t.Fatalf("dryRun cascade: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Errorf("expected DeletedCount=2 (planned), got %d", result.DeletedCount)
	}
	if result.LabelsCount < 1 {
		t.Errorf("expected at least one label counted, got %d", result.LabelsCount)
	}
	// Both rows must still be present.
	for _, id := range []string{a, b} {
		if _, err := store.GetIssue(ctx, id); err != nil {
			t.Errorf("expected %s still present after dryRun, got %v", id, err)
		}
	}
}

// 8. Issue with labels/comments/events → CASCADE deletes all child rows.
func TestDeleteIssues_CascadeChildTables(t *testing.T) {
	store, ctx := makeStore(t)
	id := makeIssue(t, store, ctx, "WithChildren")
	if err := store.AddLabel(ctx, id, "alpha", "tester"); err != nil {
		t.Fatalf("label: %v", err)
	}
	if _, err := store.AddIssueComment(ctx, id, "tester", "hi"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	// Events: at least the create-event from CreateIssue.

	// Sanity: child rows exist before delete.
	if n := countRows(t, store, ctx, "labels", "issue_id", id); n < 1 {
		t.Fatalf("expected at least one label, got %d", n)
	}
	if n := countRows(t, store, ctx, "comments", "issue_id", id); n < 1 {
		t.Fatalf("expected at least one comment, got %d", n)
	}

	if _, err := store.DeleteIssues(ctx, []string{id}, false, false, false); err != nil {
		t.Fatalf("delete: %v", err)
	}

	for _, table := range []string{"labels", "comments", "events"} {
		if n := countRows(t, store, ctx, table, "issue_id", id); n != 0 {
			t.Errorf("CASCADE did not clear %s: %d rows remain", table, n)
		}
	}
}

// 9. Cycle: A blocks B, B blocks A, cascade=true → terminates, both deleted.
func TestDeleteIssues_CascadeCycle(t *testing.T) {
	store, ctx := makeStore(t)
	a := makeIssue(t, store, ctx, "A")
	b := makeIssue(t, store, ctx, "B")
	// A→B
	makeBlocks(t, store, ctx, a, b)
	// Inserting the inverse B→A via the public API can trigger the cycle
	// check; the recursive walk's `seen` guard is what we're actually
	// testing here, so insert the back-edge with skipCycleCheck via direct
	// dependencies insert.
	_, err := store.pool.Exec(ctx,
		"INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ($1, $2, $3)",
		a, b, string(types.DepBlocks))
	if err != nil {
		t.Fatalf("insert back-edge: %v", err)
	}

	result, err := store.DeleteIssues(ctx, []string{a}, true, false, false)
	if err != nil {
		t.Fatalf("delete cycle: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Errorf("expected DeletedCount=2 (cycle resolved), got %d", result.DeletedCount)
	}
}

// 10. Wisp batch of 750 IDs → completes; per-500 batching kicks in.
func TestDeleteIssues_WispBatching(t *testing.T) {
	store, ctx := makeStore(t)
	const batchTotal = 750
	ids := make([]string, batchTotal)
	for i := 0; i < batchTotal; i++ {
		id := fmt.Sprintf("be-wisp-batch-%04d", i)
		insertWispDirect(t, store, ctx, id, fmt.Sprintf("Wisp %d", i))
		ids[i] = id
	}

	result, err := store.DeleteIssues(ctx, ids, false, false, false)
	if err != nil {
		t.Fatalf("delete batch: %v", err)
	}
	if result.DeletedCount != batchTotal {
		t.Errorf("expected DeletedCount=%d, got %d", batchTotal, result.DeletedCount)
	}

	var remaining int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM wisps WHERE id LIKE 'be-wisp-batch-%'").Scan(&remaining); err != nil {
		t.Fatalf("count wisps: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected 0 wisps remaining, got %d", remaining)
	}
}

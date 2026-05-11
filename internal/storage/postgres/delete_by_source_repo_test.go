//go:build integration_pg

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// makeIssueInRepo creates a persistent issue with source_repo set and returns
// its ID. Mirrors makeIssue (delete_test.go) but tags the source_repo column,
// which is the dimension under test here.
func makeIssueInRepo(t *testing.T, store *PostgresStore, ctx context.Context, title, sourceRepo string) string {
	t.Helper()
	issue := &types.Issue{
		Title:      title,
		IssueType:  types.TypeTask,
		Priority:   2,
		Status:     types.StatusOpen,
		SourceRepo: sourceRepo,
	}
	if err := store.CreateIssue(ctx, issue, "tester"); err != nil {
		t.Fatalf("create %q in repo %q: %v", title, sourceRepo, err)
	}
	return issue.ID
}

// 1. Empty sourceRepo → returns error, deletes nothing.
// Behavior tightening over Dolt (which silently wipes source_repo=” rows).
func TestDeleteIssuesBySourceRepo_EmptyRejected(t *testing.T) {
	store, ctx := makeStore(t)
	// Seed a couple rows so we can confirm nothing was touched.
	keep1 := makeIssueInRepo(t, store, ctx, "keep1", "~/alpha")
	keep2 := makeIssueInRepo(t, store, ctx, "keep2", "~/beta")

	n, err := store.DeleteIssuesBySourceRepo(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty sourceRepo, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty sourceRepo") {
		t.Errorf("expected guardrail error message, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted on error, got %d", n)
	}
	for _, id := range []string{keep1, keep2} {
		if _, err := store.GetIssue(ctx, id); err != nil {
			t.Errorf("expected %s still present, got %v", id, err)
		}
	}
}

// 2. No matching issues → returns (0, nil).
func TestDeleteIssuesBySourceRepo_NoMatches(t *testing.T) {
	store, ctx := makeStore(t)
	keep := makeIssueInRepo(t, store, ctx, "keep", "~/alpha")

	n, err := store.DeleteIssuesBySourceRepo(ctx, "~/no-such-repo")
	if err != nil {
		t.Fatalf("no-match: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted, got %d", n)
	}
	if _, err := store.GetIssue(ctx, keep); err != nil {
		t.Errorf("expected %s still present, got %v", keep, err)
	}
}

// 3. Mix of issues with and without children, all in one source repo →
// returns 5; CASCADE clears labels/comments/events/dependencies for them.
func TestDeleteIssuesBySourceRepo_CascadesChildren(t *testing.T) {
	store, ctx := makeStore(t)
	const repo = "~/alpha"
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		ids[i] = makeIssueInRepo(t, store, ctx, fmt.Sprintf("alpha-%d", i), repo)
	}
	// Attach a label and a comment to issue 0.
	if err := store.AddLabel(ctx, ids[0], "tagged", "tester"); err != nil {
		t.Fatalf("label: %v", err)
	}
	if _, err := store.AddIssueComment(ctx, ids[0], "tester", "hi"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	// Internal blocks edge: ids[1] blocks ids[2].
	makeBlocks(t, store, ctx, ids[1], ids[2])

	n, err := store.DeleteIssuesBySourceRepo(ctx, repo)
	if err != nil {
		t.Fatalf("delete-by-repo: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 deleted, got %d", n)
	}
	// All issues gone.
	for _, id := range ids {
		if _, err := store.GetIssue(ctx, id); err == nil {
			t.Errorf("expected %s gone", id)
		}
	}
	// CASCADE: child tables empty for the seed issues.
	for _, id := range ids {
		for _, table := range []string{"labels", "comments", "events", "dependencies"} {
			if remaining := countRows(t, store, ctx, table, "issue_id", id); remaining != 0 {
				t.Errorf("CASCADE did not clear %s for %s: %d rows remain", table, id, remaining)
			}
		}
	}
}

// 4. 1000 issues in one source repo → returns 1000, no timeout.
// Single transaction performance baseline.
func TestDeleteIssuesBySourceRepo_Bulk(t *testing.T) {
	store, ctx := makeStore(t)
	const repo = "~/bulk"
	const total = 1000
	for i := 0; i < total; i++ {
		makeIssueInRepo(t, store, ctx, fmt.Sprintf("bulk-%04d", i), repo)
	}

	n, err := store.DeleteIssuesBySourceRepo(ctx, repo)
	if err != nil {
		t.Fatalf("bulk delete: %v", err)
	}
	if n != total {
		t.Errorf("expected %d deleted, got %d", total, n)
	}
	var remaining int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM issues WHERE source_repo = $1", repo).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected 0 rows for repo %q, got %d", repo, remaining)
	}
}

// 5. One issue in repoA has an inbound dep from an issue in repoB.
// Deleting repoA removes the issue and the inbound dep edge; repoB stays.
func TestDeleteIssuesBySourceRepo_InboundDepFromOtherRepo(t *testing.T) {
	store, ctx := makeStore(t)
	const repoA = "~/alpha"
	const repoB = "~/beta"
	a := makeIssueInRepo(t, store, ctx, "in-A", repoA)
	b := makeIssueInRepo(t, store, ctx, "in-B", repoB)
	// b depends on a (a blocks b). a lives in repoA, b lives in repoB.
	makeBlocks(t, store, ctx, a, b)

	n, err := store.DeleteIssuesBySourceRepo(ctx, repoA)
	if err != nil {
		t.Fatalf("delete repoA: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted, got %d", n)
	}
	// a gone, b remains.
	if _, err := store.GetIssue(ctx, a); err == nil {
		t.Errorf("expected %s gone", a)
	}
	if _, err := store.GetIssue(ctx, b); err != nil {
		t.Errorf("expected %s still present, got %v", b, err)
	}
	// The inbound dep edge (b → a) is gone.
	if remaining := countRows(t, store, ctx, "dependencies", "issue_id", b); remaining != 0 {
		t.Errorf("expected no remaining dep edges for %s, got %d", b, remaining)
	}
	if remaining := countRows(t, store, ctx, "dependencies", "depends_on_id", a); remaining != 0 {
		t.Errorf("expected no remaining inbound dep edges to %s, got %d", a, remaining)
	}
}

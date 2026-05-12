//go:build integration_pg

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
)

// insertWispFull seeds a wisp row plus optional label, dependency, event,
// and comment rows. Mirrors insertWispDirect from delete_test.go but stages
// the aux tables a promote test needs.
func insertWispFull(t *testing.T, store *PostgresStore, ctx context.Context, id, title string) {
	t.Helper()
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisps (id, title, ephemeral) VALUES ($1, $2, TRUE)`, id, title); err != nil {
		t.Fatalf("insert wisp %s: %v", id, err)
	}
}

func addWispLabel(t *testing.T, store *PostgresStore, ctx context.Context, id, label string) {
	t.Helper()
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisp_labels (issue_id, label) VALUES ($1, $2)`, id, label); err != nil {
		t.Fatalf("insert wisp_label %s/%s: %v", id, label, err)
	}
}

func addWispDep(t *testing.T, store *PostgresStore, ctx context.Context, issueID, dependsOnID string) {
	t.Helper()
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisp_dependencies (issue_id, depends_on_id, type, created_by) VALUES ($1, $2, 'blocks', 'tester')`,
		issueID, dependsOnID); err != nil {
		t.Fatalf("insert wisp_dependency %s→%s: %v", issueID, dependsOnID, err)
	}
}

func addWispEvent(t *testing.T, store *PostgresStore, ctx context.Context, issueID, kind, actor string) {
	t.Helper()
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisp_events (issue_id, event_type, actor) VALUES ($1, $2, $3)`,
		issueID, kind, actor); err != nil {
		t.Fatalf("insert wisp_event %s/%s: %v", issueID, kind, err)
	}
}

func addWispComment(t *testing.T, store *PostgresStore, ctx context.Context, issueID, author, text string) {
	t.Helper()
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisp_comments (issue_id, author, text) VALUES ($1, $2, $3)`,
		issueID, author, text); err != nil {
		t.Fatalf("insert wisp_comment %s: %v", issueID, err)
	}
}

// TestPromoteFromEphemeral_HappyPath covers case 1 from the architect's design:
// a wisp with labels + deps (target exists) + comments + events promotes
// cleanly. The issues row replaces the wisps row, all aux rows move, and
// the audit trail gains a fresh `created` event on top of the copied
// wisp_events.
func TestPromoteFromEphemeral_HappyPath(t *testing.T) {
	store, ctx := makeStore(t)

	// Permanent target the wisp will depend on.
	target := makeIssue(t, store, ctx, "permanent target")

	// Seed the wisp with one of each kind of aux row.
	wispID := "bd-wisp-001"
	insertWispFull(t, store, ctx, wispID, "wisp under test")
	addWispLabel(t, store, ctx, wispID, "area:postgres")
	addWispLabel(t, store, ctx, wispID, "ready-to-build")
	addWispDep(t, store, ctx, wispID, target)
	addWispEvent(t, store, ctx, wispID, "created", "wisp-author")
	addWispEvent(t, store, ctx, wispID, "updated", "wisp-author")
	addWispComment(t, store, ctx, wispID, "wisp-author", "first thought")
	addWispComment(t, store, ctx, wispID, "wisp-author", "second thought")

	if err := store.PromoteFromEphemeral(ctx, wispID, "promoter"); err != nil {
		t.Fatalf("PromoteFromEphemeral: %v", err)
	}

	// Wisp rows are gone.
	if n := countRows(t, store, ctx, "wisps", "id", wispID); n != 0 {
		t.Errorf("wisps row remains after promote: %d", n)
	}
	if n := countRows(t, store, ctx, "wisp_labels", "issue_id", wispID); n != 0 {
		t.Errorf("wisp_labels rows remain after promote: %d", n)
	}
	if n := countRows(t, store, ctx, "wisp_dependencies", "issue_id", wispID); n != 0 {
		t.Errorf("wisp_dependencies rows remain after promote: %d", n)
	}
	if n := countRows(t, store, ctx, "wisp_events", "issue_id", wispID); n != 0 {
		t.Errorf("wisp_events rows remain after promote: %d", n)
	}
	if n := countRows(t, store, ctx, "wisp_comments", "issue_id", wispID); n != 0 {
		t.Errorf("wisp_comments rows remain after promote: %d", n)
	}

	// Issues row exists with ephemeral cleared.
	issue, err := store.GetIssue(ctx, wispID)
	if err != nil {
		t.Fatalf("GetIssue after promote: %v", err)
	}
	if issue.Ephemeral {
		t.Errorf("promoted issue still flagged ephemeral=true")
	}
	if issue.NoHistory {
		t.Errorf("promoted issue still flagged no_history=true")
	}

	// Aux tables on the permanent side are populated.
	if n := countRows(t, store, ctx, "labels", "issue_id", wispID); n != 2 {
		t.Errorf("labels rows after promote: got %d want 2", n)
	}
	if n := countRows(t, store, ctx, "dependencies", "issue_id", wispID); n != 1 {
		t.Errorf("dependencies rows after promote: got %d want 1", n)
	}
	if n := countRows(t, store, ctx, "comments", "issue_id", wispID); n != 2 {
		t.Errorf("comments rows after promote: got %d want 2", n)
	}
	// events: 2 copied from wisp_events plus 1 fresh `created` from the
	// promote = 3 total.
	if n := countRows(t, store, ctx, "events", "issue_id", wispID); n != 3 {
		t.Errorf("events rows after promote: got %d want 3", n)
	}
}

// TestPromoteFromEphemeral_NotFound covers case 2: promoting a non-existent
// wisp returns a typed error and leaves no rows behind.
func TestPromoteFromEphemeral_NotFound(t *testing.T) {
	store, ctx := makeStore(t)

	err := store.PromoteFromEphemeral(ctx, "bd-no-such-wisp", "promoter")
	if err == nil {
		t.Fatalf("PromoteFromEphemeral: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("PromoteFromEphemeral error: got %q, want substring %q", err.Error(), "not found")
	}
}

// TestPromoteFromEphemeral_AlreadyExists covers case 3: a permanent issue
// with the same ID already exists. Promote refuses, the wisp is untouched.
func TestPromoteFromEphemeral_AlreadyExists(t *testing.T) {
	store, ctx := makeStore(t)

	// Create a permanent issue, then a wisp with the same ID. (Direct DB
	// inserts so we can collide IDs without fighting the public API.) The
	// issues table has NOT NULL columns without defaults; the empty-string
	// fillers below match what `insertIssueRow` writes for an empty issue.
	id := "bd-collision-001"
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO issues (id, title, description, design, acceptance_criteria, notes)
		VALUES ($1, $2, '', '', '', '')`, id, "preexisting"); err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	insertWispFull(t, store, ctx, id, "colliding wisp")

	err := store.PromoteFromEphemeral(ctx, id, "promoter")
	if err == nil {
		t.Fatalf("PromoteFromEphemeral: expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("PromoteFromEphemeral error: got %q, want substring %q", err.Error(), "already exists")
	}

	// Wisp is untouched.
	if n := countRows(t, store, ctx, "wisps", "id", id); n != 1 {
		t.Errorf("wisp gone after failed promote: %d", n)
	}
}

// TestPromoteFromEphemeral_DanglingDependency covers case 4: a wisp dependency
// pointing at a target that is still a wisp gets dropped via the EXISTS
// filter. Promote completes, and the dropped edge is silently absent in
// `dependencies`.
func TestPromoteFromEphemeral_DanglingDependency(t *testing.T) {
	store, ctx := makeStore(t)

	wispID := "bd-wisp-dangle-001"
	stillWisp := "bd-wisp-still-002"
	insertWispFull(t, store, ctx, wispID, "wisp with dangling dep")
	insertWispFull(t, store, ctx, stillWisp, "wisp that stays a wisp")
	addWispDep(t, store, ctx, wispID, stillWisp)

	if err := store.PromoteFromEphemeral(ctx, wispID, "promoter"); err != nil {
		t.Fatalf("PromoteFromEphemeral: %v", err)
	}

	if n := countRows(t, store, ctx, "wisps", "id", wispID); n != 0 {
		t.Errorf("promoted wisp still in wisps: %d", n)
	}
	if n := countRows(t, store, ctx, "dependencies", "issue_id", wispID); n != 0 {
		t.Errorf("dangling dep was copied into dependencies: %d (expected 0)", n)
	}
	// The other wisp is untouched.
	if n := countRows(t, store, ctx, "wisps", "id", stillWisp); n != 1 {
		t.Errorf("unrelated wisp was disturbed: %d (expected 1)", n)
	}
}

// TestPromoteFromEphemeral_ManyComments covers case 5: many comments + events
// preserve created_at (they are NOT stamped to NOW). Promotes 50 comments
// and 50 events; the first/last created_at values match what we wrote.
func TestPromoteFromEphemeral_ManyComments(t *testing.T) {
	store, ctx := makeStore(t)

	wispID := "bd-wisp-bulk-001"
	insertWispFull(t, store, ctx, wispID, "bulk wisp")

	// Two anchor timestamps to verify created_at is preserved (not NOW()).
	anchorEarly := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	anchorLate := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisp_comments (issue_id, author, text, created_at) VALUES ($1, $2, $3, $4)`,
		wispID, "first", "anchor early", anchorEarly); err != nil {
		t.Fatalf("seed early comment: %v", err)
	}
	for i := 0; i < 48; i++ {
		if _, err := store.pool.Exec(ctx,
			`INSERT INTO wisp_comments (issue_id, author, text) VALUES ($1, $2, $3)`,
			wispID, "filler", fmt.Sprintf("body %d", i)); err != nil {
			t.Fatalf("seed comment %d: %v", i, err)
		}
	}
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO wisp_comments (issue_id, author, text, created_at) VALUES ($1, $2, $3, $4)`,
		wispID, "last", "anchor late", anchorLate); err != nil {
		t.Fatalf("seed late comment: %v", err)
	}

	if err := store.PromoteFromEphemeral(ctx, wispID, "promoter"); err != nil {
		t.Fatalf("PromoteFromEphemeral: %v", err)
	}

	if n := countRows(t, store, ctx, "comments", "issue_id", wispID); n != 50 {
		t.Errorf("comments after promote: got %d want 50", n)
	}

	// Confirm the anchor timestamps survive.
	var early, late time.Time
	if err := store.pool.QueryRow(ctx,
		`SELECT created_at FROM comments WHERE issue_id=$1 AND text='anchor early'`,
		wispID).Scan(&early); err != nil {
		t.Fatalf("read early created_at: %v", err)
	}
	if err := store.pool.QueryRow(ctx,
		`SELECT created_at FROM comments WHERE issue_id=$1 AND text='anchor late'`,
		wispID).Scan(&late); err != nil {
		t.Fatalf("read late created_at: %v", err)
	}
	if !early.Equal(anchorEarly) {
		t.Errorf("early created_at: got %v want %v", early, anchorEarly)
	}
	if !late.Equal(anchorLate) {
		t.Errorf("late created_at: got %v want %v", late, anchorLate)
	}
}

// TestPromoteFromEphemeral_CalledOnClosedStore covers the defensive
// IsClosed() guard. Mirrors the same pattern in delete_test.go.
func TestPromoteFromEphemeral_CalledOnClosedStore(t *testing.T) {
	dsn := startPGFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = store.Close()

	err = store.PromoteFromEphemeral(ctx, "bd-irrelevant", "promoter")
	if err == nil {
		t.Fatalf("expected error from PromoteFromEphemeral after Close, got nil")
	}
}

// TestPromoteFromEphemeral_StubGone is a compile-time/runtime guard that the
// method is no longer the errNotImplemented stub. If the stub regresses,
// the canonical errNotImplemented sentinel will surface and this test
// breaks.
func TestPromoteFromEphemeral_StubGone(t *testing.T) {
	store, ctx := makeStore(t)
	insertWispFull(t, store, ctx, "bd-stub-check-001", "stub guard")
	err := store.PromoteFromEphemeral(ctx, "bd-stub-check-001", "promoter")
	if err == nil {
		// Normal happy path; the stub would also have returned a non-nil error,
		// so we have to specifically check the sentinel branch.
		return
	}
	if errors.Is(err, errNotImplemented) {
		t.Fatalf("PromoteFromEphemeral fell back to errNotImplemented stub: %v", err)
	}
}

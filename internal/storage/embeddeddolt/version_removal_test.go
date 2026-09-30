//go:build cgo

package embeddeddolt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// removeVersionRaw runs issueops.RemoveVersionInTx in its own transaction on a
// short-lived raw connection, the same way testEnv.exec issues raw writes. The
// removal is only the SQL half (RemoveVersionInTx never stages or commits the
// Dolt working set), so committing the SQL transaction is all a caller owes.
func removeVersionRaw(ctx context.Context, te *testEnv, issueID string, revision int64, reason string) error {
	db, cleanup, err := embeddeddolt.OpenSQL(ctx, te.dataDir, te.database, "main")
	if err != nil {
		return err
	}
	defer func() { _ = cleanup() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := issueops.RemoveVersionInTx(ctx, tx, issueID, revision, reason); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// TestVersionRemovalSoftRemovesOnlyTheNamedVersionAndTheFirstRemovalWins is the
// embedded-Dolt twin of the dolt leg's test of the same name: the same effects,
// checked against the embedded engine and its driver instead of a Dolt server,
// because the two drivers are exactly where a SQL-level assumption (how a
// boolean expression scans, what an UPDATE reports) could differ.
func TestVersionRemovalSoftRemovesOnlyTheNamedVersionAndTheFirstRemovalWins(t *testing.T) {
	skipUnlessEmbeddedDolt(t)
	te := newTestEnv(t, "vrem")
	ctx := t.Context()

	configurer, ok := any(te.store).(storage.VersionedHistoryConfigurer)
	if !ok {
		t.Fatalf("%T does not implement storage.VersionedHistoryConfigurer", te.store)
	}
	configurer.SetVersionedHistoryEnabled(true)
	defer configurer.SetVersionedHistoryEnabled(false)

	const id = "vrem-1"
	if err := te.store.CreateIssue(ctx, &types.Issue{
		ID: id, Title: "first", IssueType: types.TypeTask, Status: types.StatusOpen,
	}, "actor"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	// A real change, so this mints revision 2 rather than being discarded as
	// a no-op: the sibling revision is what proves the removal is selective.
	if err := te.store.UpdateIssue(ctx, id, map[string]any{"title": "second"}, "actor"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	// removal reads the marker back as text so the comparison below never
	// depends on how a driver parses DATETIME(6).
	removal := func(revision int64) (removedAt, reason string) {
		t.Helper()
		te.queryScalar(t, ctx,
			`SELECT COALESCE(CAST(removed_at AS CHAR), ''), COALESCE(removed_reason, '')
			   FROM issue_versions WHERE issue_id = ? AND revision = ?`,
			[]any{id, revision}, &removedAt, &reason)
		return removedAt, reason
	}

	for _, revision := range []int64{1, 2} {
		if at, reason := removal(revision); at != "" || reason != "" {
			t.Fatalf("revision %d starts removed (removed_at=%q removed_reason=%q), want a live version", revision, at, reason)
		}
	}

	if err := removeVersionRaw(ctx, te, id, 1, issueops.VersionRemovalReasonReorganization); err != nil {
		t.Fatalf("RemoveVersionInTx(rev 1, reorganization) = %v, want nil", err)
	}
	firstAt, firstReason := removal(1)
	if firstAt == "" || firstReason != issueops.VersionRemovalReasonReorganization {
		t.Fatalf("revision 1 after removal: removed_at=%q removed_reason=%q, want a timestamp and %q", firstAt, firstReason, issueops.VersionRemovalReasonReorganization)
	}
	if at, reason := removal(2); at != "" || reason != "" {
		t.Errorf("removing revision 1 also marked revision 2 (removed_at=%q removed_reason=%q), want it untouched", at, reason)
	}

	// First removal wins: a later removal, whatever its reason, changes neither
	// the recorded time nor the recorded reason -- and is not an error.
	if err := removeVersionRaw(ctx, te, id, 1, issueops.VersionRemovalReasonErasure); err != nil {
		t.Errorf("removing an already-removed version = %v, want nil (a no-op)", err)
	}
	if at, reason := removal(1); at != firstAt || reason != firstReason {
		t.Errorf("second removal rewrote the marker: removed_at %q -> %q, removed_reason %q -> %q; the first removal must win", firstAt, at, firstReason, reason)
	}

	if err := removeVersionRaw(ctx, te, id, 99, issueops.VersionRemovalReasonReorganization); !errors.Is(err, issueops.ErrVersionNotFound) {
		t.Errorf("removing a revision that was never minted = %v, want an error wrapping ErrVersionNotFound", err)
	}
	if err := removeVersionRaw(ctx, te, id, 2, "because-i-said-so"); err == nil {
		t.Error("removing with a reason outside the vocabulary = nil, want an error")
	}
	if at, reason := removal(2); at != "" || reason != "" {
		t.Errorf("a rejected removal still marked revision 2 (removed_at=%q removed_reason=%q)", at, reason)
	}

	// Soft removal: every minted row is still there.
	var rows int
	te.queryScalar(t, ctx, `SELECT COUNT(*) FROM issue_versions WHERE issue_id = ?`, []any{id}, &rows)
	if rows != 2 {
		t.Errorf("issue_versions rows for %s = %d after removal, want 2: removal must keep the row as durable evidence, not delete it", id, rows)
	}
}

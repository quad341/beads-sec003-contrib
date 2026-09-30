package dolt

import (
	"errors"
	"testing"

	"github.com/steveyegge/beads/backend/conformance"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// TestVersionRemovalReasonsMatchConformance pins the strings issueops writes
// to issue_versions.removed_reason against the vocabulary
// backend/conformance.RemovalReason spells. issueops cannot import the
// conformance package (production code must not depend on a test-support
// package, and the two would form an import cycle), so this leg -- which
// imports both -- is where the two spellings are held together.
func TestVersionRemovalReasonsMatchConformance(t *testing.T) {
	for reason, got := range map[conformance.RemovalReason]string{
		conformance.RemovalReasonRetention:      issueops.VersionRemovalReasonRetention,
		conformance.RemovalReasonErasure:        issueops.VersionRemovalReasonErasure,
		conformance.RemovalReasonReorganization: issueops.VersionRemovalReasonReorganization,
	} {
		if want := reason.String(); got != want {
			t.Errorf("issueops removal reason for conformance.RemovalReason(%d) = %q, want %q", int(reason), got, want)
		}
	}
}

// TestVersionRemovalSoftRemovesOnlyTheNamedVersionAndTheFirstRemovalWins runs
// issueops.RemoveVersionInTx against a real Dolt server, pinning the effects
// the SQL-contract tests in issueops cannot: the named row is stamped and
// KEPT (a resolver needs it to answer "gone" rather than "unknown"), its
// sibling revision is untouched, a second removal never rewrites why or when
// the first one happened, and a bad request writes nothing.
func TestVersionRemovalSoftRemovesOnlyTheNamedVersionAndTheFirstRemovalWins(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()
	ctx, cancel := testContext(t)
	defer cancel()

	configurer, ok := any(store).(storage.VersionedHistoryConfigurer)
	if !ok {
		t.Fatalf("%T does not implement storage.VersionedHistoryConfigurer", store)
	}
	configurer.SetVersionedHistoryEnabled(true)
	defer configurer.SetVersionedHistoryEnabled(false)

	const id = "vrem-1"
	if err := store.CreateIssue(ctx, &types.Issue{
		ID: id, Title: "first", IssueType: types.TypeTask, Status: types.StatusOpen,
	}, "actor"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	// A real change, so this mints revision 2 rather than being discarded as
	// a no-op: the sibling revision is what proves the removal is selective.
	if err := store.UpdateIssue(ctx, id, map[string]any{"title": "second"}, "actor"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	// The store holds one connection (see setupTestStore), so every step
	// below runs to completion before the next begins.
	remove := func(revision int64, reason string) error {
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx: %v", err)
		}
		if err := issueops.RemoveVersionInTx(ctx, tx, id, revision, reason); err != nil {
			return errors.Join(err, tx.Rollback())
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		return nil
	}
	// removal reads the marker back as text so the comparison below never
	// depends on how a driver parses DATETIME(6).
	removal := func(revision int64) (removedAt, reason string) {
		t.Helper()
		if err := store.db.QueryRowContext(ctx,
			`SELECT COALESCE(CAST(removed_at AS CHAR), ''), COALESCE(removed_reason, '')
			   FROM issue_versions WHERE issue_id = ? AND revision = ?`, id, revision,
		).Scan(&removedAt, &reason); err != nil {
			t.Fatalf("read removal marker for %s@%d: %v", id, revision, err)
		}
		return removedAt, reason
	}

	for _, revision := range []int64{1, 2} {
		if at, reason := removal(revision); at != "" || reason != "" {
			t.Fatalf("revision %d starts removed (removed_at=%q removed_reason=%q), want a live version", revision, at, reason)
		}
	}

	if err := remove(1, issueops.VersionRemovalReasonReorganization); err != nil {
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
	if err := remove(1, issueops.VersionRemovalReasonErasure); err != nil {
		t.Errorf("removing an already-removed version = %v, want nil (a no-op)", err)
	}
	if at, reason := removal(1); at != firstAt || reason != firstReason {
		t.Errorf("second removal rewrote the marker: removed_at %q -> %q, removed_reason %q -> %q; the first removal must win", firstAt, at, firstReason, reason)
	}

	if err := remove(99, issueops.VersionRemovalReasonReorganization); !errors.Is(err, issueops.ErrVersionNotFound) {
		t.Errorf("removing a revision that was never minted = %v, want an error wrapping ErrVersionNotFound", err)
	}
	if err := remove(2, "because-i-said-so"); err == nil {
		t.Error("removing with a reason outside the vocabulary = nil, want an error")
	}
	if at, reason := removal(2); at != "" || reason != "" {
		t.Errorf("a rejected removal still marked revision 2 (removed_at=%q removed_reason=%q)", at, reason)
	}

	// Soft removal: every minted row is still there.
	var rows int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM issue_versions WHERE issue_id = ?`, id,
	).Scan(&rows); err != nil {
		t.Fatalf("count issue_versions rows: %v", err)
	}
	if rows != 2 {
		t.Errorf("issue_versions rows for %s = %d after removal, want 2: removal must keep the row as durable evidence, not delete it", id, rows)
	}
}

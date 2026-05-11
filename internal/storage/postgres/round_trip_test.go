//go:build integration_pg

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// TestPgSyncRoundTrip exercises the full pg-sync flow end-to-end through the
// storage interface — the acceptance test for be-yl8wc4 (FR-01/FR-02/FR-04):
//
//  1. openStore (≈ bd init --backend=postgres) runs migrations to v3.
//  2. SetConfig issue_prefix establishes the prefix.
//  3. SetConfig types.custom=["convoy","agent"] writes the config row AND
//     populates custom_types atomically via the dispatch in be-yl8wc4.3 /
//     be-yl8wc4.2.
//  4. CreateIssue with type=convoy succeeds — loadCustomConfigInTx sees the
//     just-populated row, and ValidateWithCustom accepts the type.
//  5. CreateIssue with type=invalidtype fails with "invalid issue type" —
//     proving the custom_types list is authoritative, not "anything goes".
//
// This is also the smoke test for the dolt-free path: the file imports nothing
// from internal/storage/dolt or github.com/dolthub/* and the test can be run
// under `-tags 'gms_pure_go integration_pg no_dolt'` to confirm the pg
// backend works without the dolt code path compiled in.
func TestPgSyncRoundTrip(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Step 1 — open.
	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	t.Log("step 1: openStore + migrations OK")

	// Step 2 — prefix.
	if err := store.SetConfig(ctx, "issue_prefix", "bd"); err != nil {
		t.Fatalf("set issue_prefix: %v", err)
	}
	t.Log("step 2: SetConfig issue_prefix=bd OK")

	// Step 3 — custom types via the dispatch.
	if err := store.SetConfig(ctx, "types.custom", `["convoy","agent"]`); err != nil {
		t.Fatalf("set types.custom: %v", err)
	}
	customTypes, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get custom types: %v", err)
	}
	if !equalStrings(customTypes, []string{"agent", "convoy"}) {
		t.Fatalf("step 3: custom_types after SetConfig: got %v, want [agent convoy]", customTypes)
	}
	t.Log("step 3: SetConfig types.custom synchronized to custom_types OK")

	// Step 4 — happy path: convoy is a custom type, so CreateIssue must accept it.
	good := &types.Issue{
		Title:     "First convoy",
		IssueType: types.IssueType("convoy"),
		Priority:  1,
		Status:    types.StatusOpen,
	}
	if err := store.CreateIssue(ctx, good, "tester"); err != nil {
		t.Fatalf("create convoy issue: %v", err)
	}
	if good.ID == "" {
		t.Fatal("convoy issue created with empty ID")
	}
	got, err := store.GetIssue(ctx, good.ID)
	if err != nil {
		t.Fatalf("read back convoy issue: %v", err)
	}
	if got.IssueType != types.IssueType("convoy") {
		t.Errorf("step 4: created issue IssueType: got %q, want %q", got.IssueType, "convoy")
	}
	t.Logf("step 4: CreateIssue(type=convoy) succeeded, id=%s, type=%s", got.ID, got.IssueType)

	// Step 5 — negative control: an unknown type must be rejected.
	bad := &types.Issue{
		Title:     "Should be rejected",
		IssueType: types.IssueType("invalidtype"),
		Priority:  2,
		Status:    types.StatusOpen,
	}
	err = store.CreateIssue(ctx, bad, "tester")
	if err == nil {
		t.Fatal("step 5: expected error for type=invalidtype, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid issue type") {
		t.Errorf("step 5: error message should mention 'invalid issue type', got: %q", err.Error())
	}
	t.Logf("step 5: CreateIssue(type=invalidtype) rejected as expected: %v", err)
}

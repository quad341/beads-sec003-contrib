//go:build cgo && dolt_only

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/tracker"
	"github.com/steveyegge/beads/internal/types"
)

// TestJiraSyncFilterIsAlwaysFull is the be-uwvs.5 round-trip gate for
// the Jira sync path. Asserts that the sync engine's SearchIssues calls
// (internal/tracker/engine.go:246, :315, :810, :1246) never carry
// Lite=true.
//
// The Jira sync engine reads local issues — including their external_ref
// fields and full body — to detect conflicts and prepare push payloads.
// A lite-shaped scan would silently drop the description/notes/etc from
// the push payload, surfacing as truncated Jira issues on the other end.
//
// Path under test: tracker.Engine.DetectConflicts at engine.go:226. This
// is the canonical example of the round-trip scan pattern used through-
// out engine.go — all four SearchIssues call sites in that file use the
// same zero-value filter. Gating DetectConflicts gates the pattern.
//
// We construct a minimal stubTracker satisfying the tracker.IssueTracker
// interface (13 methods) rather than wiring a real Jira tracker. The
// tracker's only role in DetectConflicts is to scope external-ref
// matching; returning IsExternalRef=true makes every seeded row match
// and gives the gate a non-trivial code path to exercise.
//
// Build tags: cgo && dolt_only matches existing cgo cmd/bd tests.
func TestJiraSyncFilterIsAlwaysFull(t *testing.T) {
	if testDoltServerPort == 0 {
		t.Skip("Dolt test server not available")
	}
	if testutil.DoltContainerCrashed() {
		t.Skipf("Dolt test server crashed: %v", testutil.DoltContainerCrashError())
	}

	ensureTestMode(t)
	saved := saveAndRestoreGlobals(t)
	_ = saved

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	dbName := uniqueTestDBName(t)
	testDBPath := filepath.Join(beadsDir, "dolt")
	writeTestMetadata(t, testDBPath, dbName)
	inner := newTestStore(t, testDBPath)
	spy := newFilterCapturingStore(inner)
	store = spy
	storeMutex.Lock()
	storeActive = true
	storeMutex.Unlock()
	t.Cleanup(func() {
		store = nil
		storeMutex.Lock()
		storeActive = false
		storeMutex.Unlock()
	})

	ctx := context.Background()
	rootCtx = ctx

	// last_sync seeded so DetectConflicts proceeds past the early
	// "no previous sync" return at engine.go:235. The actual value
	// doesn't matter — as long as the metadata key exists and parses,
	// the function flows into the SearchIssues path.
	if err := inner.SetLocalMetadata(ctx, "stub.last_sync", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SetLocalMetadata: %v", err)
	}

	// Seed one row with an external ref so DetectConflicts has
	// something to consider (defending against an early bail-out that
	// would skip the spy capture).
	if _, err := inner.DB().ExecContext(ctx, `INSERT INTO issues (id, title, status, priority, issue_type, external_ref) VALUES (?, ?, ?, ?, ?, ?)`,
		"jira-1", "Jira gate seed", "open", 1, "task", "https://stub.test/EXT-1"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	engine := tracker.NewEngine(&stubTracker{}, spy, "tester")
	if _, err := engine.DetectConflicts(ctx); err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if spy.Calls() == 0 {
		t.Fatal("DetectConflicts did not invoke SearchIssues; gate did not exercise the filter")
	}

	for i, f := range spy.AllFilters() {
		if f.Lite {
			t.Errorf("be-uwvs.5 gate: SearchIssues call %d had filter.Lite=true; jira sync must use the full-shape SELECT (push payload integrity)", i)
		}
	}

	last := spy.LastFilter()
	if last.Limit != 0 {
		t.Errorf("be-uwvs.5 gate: jira sync final filter had Limit=%d; want 0 (sync must scan every external-refed row)", last.Limit)
	}
}

// stubTracker satisfies tracker.IssueTracker with no-op behavior. It
// exists solely to feed tracker.NewEngine in the lite-gate test; only
// the methods DetectConflicts actually touches (DisplayName,
// ConfigPrefix, IsExternalRef) produce non-trivial values. Every other
// method is a tracker-side no-op so the spy's storage-side capture is
// the only side effect of running DetectConflicts.
type stubTracker struct{}

func (*stubTracker) Name() string                                    { return "stub" }
func (*stubTracker) DisplayName() string                             { return "Stub" }
func (*stubTracker) ConfigPrefix() string                            { return "stub" }
func (*stubTracker) Init(_ context.Context, _ storage.Storage) error { return nil }
func (*stubTracker) Validate() error                                 { return nil }
func (*stubTracker) Close() error                                    { return nil }
func (*stubTracker) FieldMapper() tracker.FieldMapper                { return nil }
func (*stubTracker) IsExternalRef(_ string) bool                     { return true }
func (*stubTracker) ExtractIdentifier(_ string) string               { return "" }
func (*stubTracker) BuildExternalRef(_ *tracker.TrackerIssue) string { return "" }

func (*stubTracker) FetchIssues(_ context.Context, _ tracker.FetchOptions) ([]tracker.TrackerIssue, error) {
	return nil, nil
}
func (*stubTracker) FetchIssue(_ context.Context, _ string) (*tracker.TrackerIssue, error) {
	return nil, nil
}
func (*stubTracker) CreateIssue(_ context.Context, _ *types.Issue) (*tracker.TrackerIssue, error) {
	return nil, nil
}
func (*stubTracker) UpdateIssue(_ context.Context, _ string, _ *types.Issue) (*tracker.TrackerIssue, error) {
	return nil, nil
}

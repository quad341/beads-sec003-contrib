//go:build cgo && dolt_only

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/testutil"
)

// TestMigrateIssuesFilterIsAlwaysFull is the be-uwvs.5 round-trip gate
// for `bd migrate-issues`. Asserts that the migration's candidate-find
// filter — built in findCandidateIssues at cmd/bd/migrate_issues.go:269
// — never carries Lite=true. PG↔Dolt round-trip migrations would
// silently lose the heavy text body if the candidate scan ran lite.
//
// Why findCandidateIssues and not executeMigrateIssues end-to-end:
//
//   - executeMigrateIssues also calls validateRepos (lines 233/254),
//     which intentionally uses Limit=1 for existence probing. Those
//     calls do not round-trip data and are out of scope for the
//     "Limit=0 on round-trip" half of the gate.
//   - findCandidateIssues IS the round-trip candidate scan. Its filter
//     has Limit=0 (no cap, by design) and Lite=false (zero-value).
//   - Testing the candidate-find path directly isolates the property
//     under test from migration plumbing (orphan checks, plan display,
//     confirmation gate) that the gate doesn't care about.
//
// If a future refactor moves the candidate-find SearchIssues call
// into a different function, the gate's import line will fail to
// compile — that's a load-bearing signal to re-target the test.
//
// Build tags: cgo && dolt_only matches existing cgo cmd/bd tests.
func TestMigrateIssuesFilterIsAlwaysFull(t *testing.T) {
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

	// Seed one row in the source repo so findCandidateIssues has
	// something to find. The migration semantics use SourceRepo as the
	// filter discriminator.
	if _, err := inner.DB().ExecContext(ctx, `INSERT INTO issues (id, title, description, status, priority, issue_type, source_repo) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"mig-1", "Migration gate seed", "heavy", "open", 1, "task", "source-repo-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	params := migrateIssuesParams{
		from:     "source-repo-a",
		to:       "source-repo-b",
		priority: -1, // sentinel: "any priority"
	}
	if _, err := findCandidateIssues(ctx, spy, params); err != nil {
		t.Fatalf("findCandidateIssues: %v", err)
	}

	if spy.Calls() == 0 {
		t.Fatal("findCandidateIssues did not invoke SearchIssues; gate did not exercise the filter")
	}

	for i, f := range spy.AllFilters() {
		if f.Lite {
			t.Errorf("be-uwvs.5 gate: SearchIssues call %d had filter.Lite=true; bd migrate-issues must use the full-shape SELECT (cross-backend round-trip)", i)
		}
	}

	last := spy.LastFilter()
	if last.Limit != 0 {
		t.Errorf("be-uwvs.5 gate: findCandidateIssues final filter had Limit=%d; want 0 (the candidate scan must see every matching row)", last.Limit)
	}
}

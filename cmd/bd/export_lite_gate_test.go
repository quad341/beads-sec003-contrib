//go:build cgo && dolt_only

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/testutil"
)

// TestExportFilterIsAlwaysFull is the be-uwvs.5 round-trip gate for
// `bd export`. Asserts that runExport never constructs an IssueFilter
// with Lite=true or Limit!=0 — both would corrupt the export by
// dropping the heavy text body (Lite=true: blank descriptions in
// JSONL; Limit>0: silent row truncation past the cap).
//
// Today's filter construction lives at cmd/bd/export.go:92-125. The
// builder for be-uwvs.5 should change nothing in export.go for this
// gate to pass — runExport already constructs the filter without
// touching .Lite, and the zero-value default keeps it false. The
// failure mode this gate prevents:
//
//   - A future "smart export" change defaults filter.Lite=true under
//     some flag, breaking restoration from the JSONL backup.
//   - A future "rate-limited export" change defaults filter.Limit to
//     a non-zero cap, breaking large-rig migrations.
//
// Build tags: cgo && dolt_only matches existing cgo cmd/bd tests
// (export_test.go:1). Inherits the pre-existing test_helpers_pure_test
// vs test_helpers_test redeclaration on this branch — separate audit
// bug (be-7yt fallout), tracked outside this bead.
func TestExportFilterIsAlwaysFull(t *testing.T) {
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

	// Seed one row so runExport has something to iterate (defending
	// against an early-return bail-out that skips the spy capture).
	if _, err := inner.DB().ExecContext(ctx, `INSERT INTO issues (id, title, description, design, acceptance_criteria, notes, status, priority, issue_type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"gate-1", "Lite-gate seed", "heavy-body", "", "", "", "open", 1, "task"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	exportOutput = filepath.Join(tmpDir, "out.jsonl")
	exportAll = false
	exportIncludeInfra = false
	exportScrub = false
	exportIncludeMemories = false
	exportNoMemories = false
	t.Cleanup(func() { exportOutput = "" })

	if err := runExport(nil, nil); err != nil {
		t.Fatalf("runExport: %v", err)
	}

	if spy.Calls() == 0 {
		t.Fatal("runExport did not invoke SearchIssues; gate did not exercise the filter")
	}

	for i, f := range spy.AllFilters() {
		if f.Lite {
			t.Errorf("be-uwvs.5 gate: SearchIssues call %d had filter.Lite=true; export must use the full-shape SELECT (round-trip integrity)", i)
		}
	}

	last := spy.LastFilter()
	if last.Limit != 0 {
		t.Errorf("be-uwvs.5 gate: bd export final filter had Limit=%d; want 0 (unlimited — export is a backup tool, no row cap)", last.Limit)
	}
}

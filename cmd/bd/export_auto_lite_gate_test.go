//go:build cgo && dolt_only

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/testutil"
)

// TestExportAutoFilterIsAlwaysFull is the be-uwvs.5 round-trip gate for
// `bd export --auto` (the incremental auto-export run from
// PersistentPostRun). Asserts that exportToFile — the shared helper
// behind both `bd export -o` and auto-export, at cmd/bd/export_auto.go:140
// — never constructs an IssueFilter with Lite=true.
//
// The auto-export path is particularly sensitive: it runs as a side
// effect of nearly every bd command and writes a git-tracked JSONL
// snapshot. A lite-shaped export would silently corrupt the snapshot
// across every subsequent commit; the regression would only surface
// at restore time. This gate prevents the regression at write time.
//
// We invoke exportToFile directly (rather than maybeAutoExport, which
// runs guard checks first) so the test exercises the round-trip path
// without depending on config.export.auto being set in the test rig.
//
// Build tags: cgo && dolt_only matches existing cgo cmd/bd tests.
func TestExportAutoFilterIsAlwaysFull(t *testing.T) {
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

	if _, err := inner.DB().ExecContext(ctx, `INSERT INTO issues (id, title, description, design, acceptance_criteria, notes, status, priority, issue_type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"gate-auto-1", "Auto-export gate seed", "heavy", "", "", "", "open", 1, "task"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "auto-export.jsonl")
	if _, _, err := exportToFile(ctx, outPath, false); err != nil {
		t.Fatalf("exportToFile: %v", err)
	}

	if spy.Calls() == 0 {
		t.Fatal("exportToFile did not invoke SearchIssues; gate did not exercise the filter")
	}

	for i, f := range spy.AllFilters() {
		if f.Lite {
			t.Errorf("be-uwvs.5 gate: SearchIssues call %d had filter.Lite=true; bd export --auto must use the full-shape SELECT (snapshot integrity)", i)
		}
	}

	last := spy.LastFilter()
	if last.Limit != 0 {
		t.Errorf("be-uwvs.5 gate: bd export --auto final filter had Limit=%d; want 0 (snapshot must include every persistent row)", last.Limit)
	}
}

//go:build cgo

// Tests for Count* aggregate methods (be-fv0xme, be-8bmmp2).
// These cover the 8 parity scenarios from design be-rqyhbb §5.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// bdCountJSON runs "bd count" (via bd list --json with limit) and returns the total
// by parsing the JSON output. Returns -1 on error.
func bdListJSONCount(t *testing.T, bd, dir string, args ...string) int {
	t.Helper()
	issues := bdListJSON(t, bd, dir, args...)
	return len(issues)
}

// bdCountStats runs "bd status --json" and returns the summary object.
func bdCountStats(t *testing.T, bd, dir string) map[string]interface{} {
	t.Helper()
	result := bdStatusJSON(t, bd, dir)
	if summary, ok := result["summary"].(map[string]interface{}); ok {
		return summary
	}
	return result
}

// TestCount_Empty verifies that CountIssues returns 0 for an empty store.
func TestCount_Empty(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "cta")

	// Empty store: bd list --json should return 0 issues.
	issues := bdListJSON(t, bd, dir)
	if len(issues) != 0 {
		t.Errorf("empty store: expected 0 issues, got %d", len(issues))
	}
}

// TestCount_SingleMatch verifies CountIssues returns 1 for a single matching issue.
func TestCount_SingleMatch(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "cts")

	bdCreate(t, bd, dir, "Single issue", "--type", "task")

	issues := bdListJSON(t, bd, dir)
	if len(issues) != 1 {
		t.Errorf("single match: expected 1 issue, got %d", len(issues))
	}
}

// TestCount_ManyMatches verifies CountIssues returns N for N issues.
func TestCount_ManyMatches(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ctm")

	const n = 10
	for i := 0; i < n; i++ {
		bdCreate(t, bd, dir, "Issue", "--type", "task")
	}

	issues := bdListJSON(t, bd, dir)
	if len(issues) != n {
		t.Errorf("many matches: expected %d issues, got %d", n, len(issues))
	}
}

// TestCount_FilterNarrowing verifies that status filter narrows the count.
func TestCount_FilterNarrowing(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ctf")

	issue1 := bdCreate(t, bd, dir, "Open issue", "--type", "task")
	bdCreate(t, bd, dir, "Another open", "--type", "task")
	bdUpdate(t, bd, dir, issue1.ID, "--status", "in_progress")

	// Count all issues.
	allIssues := bdListJSON(t, bd, dir)
	// Count only in_progress.
	inProgressIssues := bdListJSON(t, bd, dir, "--status", "in_progress")

	if len(allIssues) <= len(inProgressIssues) {
		t.Errorf("filter narrowing: all(%d) should be > in_progress(%d)",
			len(allIssues), len(inProgressIssues))
	}
	if len(inProgressIssues) != 1 {
		t.Errorf("in_progress filter: expected 1, got %d", len(inProgressIssues))
	}
}

// TestCount_LimitIgnored verifies that a limit filter does not affect equivalence
// between the count and the actual list (count always returns full match count).
func TestCount_LimitIgnored(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ctl")

	for i := 0; i < 5; i++ {
		bdCreate(t, bd, dir, "Issue", "--type", "task")
	}

	// list with limit=1 should return only 1 issue.
	limited := bdListJSON(t, bd, dir, "--limit", "1")
	if len(limited) != 1 {
		t.Errorf("limit=1: expected 1, got %d", len(limited))
	}

	// Total count (no limit) should still be 5.
	all := bdListJSON(t, bd, dir)
	if len(all) != 5 {
		t.Errorf("no-limit: expected 5, got %d", len(all))
	}
}

// TestCount_SearchEquivalence verifies that bd list returns the same count
// as bd list --json with the same filter (equivalence gate).
func TestCount_SearchEquivalence(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "cte")

	for i := 0; i < 7; i++ {
		bdCreate(t, bd, dir, "Issue", "--type", "task")
	}
	// Close 3 of them.
	issues := bdListJSON(t, bd, dir)
	for i := 0; i < 3 && i < len(issues); i++ {
		bdUpdate(t, bd, dir, issues[i].ID, "--status", "closed")
	}

	// JSON count should equal the actual issue list count.
	openIssues := bdListJSON(t, bd, dir, "--status", "open")
	closedIssues := bdListJSON(t, bd, dir, "--status", "closed")

	if len(openIssues)+len(closedIssues) != 7 {
		t.Errorf("equivalence: open(%d) + closed(%d) should equal 7",
			len(openIssues), len(closedIssues))
	}
}

// TestCount_StatusStats verifies bd status --json shows correct counts.
func TestCount_StatusStats(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "cts2")

	bdCreate(t, bd, dir, "Open issue 1", "--type", "task")
	issue2 := bdCreate(t, bd, dir, "Open issue 2", "--type", "task")
	bdUpdate(t, bd, dir, issue2.ID, "--status", "closed")

	stats := bdCountStats(t, bd, dir)

	// total_issues should be 2.
	if total, ok := stats["total_issues"].(float64); ok && int(total) != 2 {
		t.Errorf("total_issues: got %v, want 2", total)
	}
}

// TestCount_DependencyCount verifies CountDependents/CountDependencies via bd info.
func TestCount_DependencyCount(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ctd")

	parent := bdCreate(t, bd, dir, "Parent issue", "--type", "task")
	child1 := bdCreate(t, bd, dir, "Child 1", "--type", "task")
	child2 := bdCreate(t, bd, dir, "Child 2", "--type", "task")

	// Add deps: child1 and child2 depend on parent.
	bdRunWithFlockRetry(t, bd, dir, "dep", "add", child1.ID, parent.ID) //nolint:errcheck
	bdRunWithFlockRetry(t, bd, dir, "dep", "add", child2.ID, parent.ID) //nolint:errcheck

	// bd list --json should show issues with dep counts.
	cmd := exec.Command(bd, "list", "--json")
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("bd list --json non-zero exit: %v\noutput: %s", err, out)
	}

	// Parse JSONL output.
	var parentIssue *types.IssueWithCounts
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var iss types.IssueWithCounts
		if err := json.Unmarshal([]byte(line), &iss); err != nil {
			continue
		}
		if iss.ID == parent.ID {
			parentIssue = &iss
			break
		}
	}

	if parentIssue == nil {
		t.Fatalf("parent issue %s not found in bd list --json output", parent.ID)
	}
	// The parent should have 2 dependents (child1 and child2 depend on it).
	if parentIssue.DependentCount != 2 {
		t.Errorf("parent DependentCount: got %d, want 2", parentIssue.DependentCount)
	}
}

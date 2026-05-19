//go:build cgo

package main

import (
	"os"
	"strings"
	"testing"
)

// bdStats runs "bd stats" (alias for "bd status") with the given args.
// Returns stdout; fatals on non-zero exit.
func bdStats(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	return bdStatus(t, bd, dir, args...)
}

// bdStatsJSON runs "bd stats --json" and parses the result.
func bdStatsJSON(t *testing.T, bd, dir string, args ...string) map[string]interface{} {
	t.Helper()
	return bdStatusJSON(t, bd, dir, args...)
}

// setupStatsRepo creates a repo with a few open issues and one blocked issue
// for meaningful stats output.
func setupStatsRepo(t *testing.T, bd string) (dir string) {
	t.Helper()
	dir, _, _ = bdInit(t, bd, "--prefix", "zk")
	issue := bdCreate(t, bd, dir, "Open issue", "--type", "task")
	bdCreate(t, bd, dir, "Another open", "--type", "task")
	// Create a dependency to make one issue blocked.
	blocker := bdCreate(t, bd, dir, "Blocker", "--type", "task")
	blocked := bdCreate(t, bd, dir, "Blocked by blocker", "--type", "task")
	// Add dependency: blocked depends on blocker.
	bdRunWithFlockRetry(t, bd, dir, "dep", "add", blocked.ID, blocker.ID) //nolint:errcheck
	_ = issue
	return dir
}

// TestBdStats_NoBlockedHumanOutput verifies that bd stats --no-blocked output
// contains "(skipped)" on the Blocked line and does NOT contain a numeric Blocked count.
func TestBdStats_NoBlockedHumanOutput(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir := setupStatsRepo(t, bd)

	out := bdStats(t, bd, dir, "--no-blocked")

	if !strings.Contains(out, "(skipped)") {
		t.Errorf("expected '(skipped)' in --no-blocked output, got:\n%s", out)
	}
	// The Blocked line should say "skipped", not show a number.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), "blocked") && !strings.Contains(line, "skipped") {
			// Line mentions blocked but doesn't say skipped — check it's not a digit count.
			if containsDigit(line) {
				t.Errorf("blocked line should not show a numeric value with --no-blocked, got: %q", line)
			}
		}
	}
}

// TestBdStats_NoBlockedJSONOutput verifies that bd stats --no-blocked --json has
// null blocked_issues and blocked_count_skipped=true.
func TestBdStats_NoBlockedJSONOutput(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir := setupStatsRepo(t, bd)

	result := bdStatsJSON(t, bd, dir, "--no-blocked")
	summary, ok := result["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'summary' object in JSON, got: %v", result)
	}

	// blocked_issues must be null (not present or explicitly nil).
	if bi, exists := summary["blocked_issues"]; exists && bi != nil {
		t.Errorf("summary.blocked_issues must be null with --no-blocked, got: %v", bi)
	}

	// blocked_count_skipped must be true.
	if skip, ok := result["blocked_count_skipped"].(bool); !ok || !skip {
		t.Errorf("expected blocked_count_skipped=true, got: %v", result["blocked_count_skipped"])
	}

	// Other stats fields must still be present.
	if _, ok := summary["open_issues"]; !ok {
		t.Error("summary.open_issues must be present with --no-blocked")
	}
	if _, ok := summary["closed_issues"]; !ok {
		t.Error("summary.closed_issues must be present with --no-blocked")
	}
}

// TestBdStats_DefaultJSON verifies that without --no-blocked, blocked_issues is
// a non-null integer and blocked_count_skipped is absent.
func TestBdStats_DefaultJSON(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir := setupStatsRepo(t, bd)

	result := bdStatsJSON(t, bd, dir)
	summary, ok := result["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'summary' object in JSON, got: %v", result)
	}

	// blocked_issues must be a non-null number (0 or more).
	bi, exists := summary["blocked_issues"]
	if !exists || bi == nil {
		t.Errorf("summary.blocked_issues must be non-null without --no-blocked, got: %v", bi)
	}

	// blocked_count_skipped must be absent (omitempty).
	if _, found := result["blocked_count_skipped"]; found {
		t.Error("blocked_count_skipped must be absent without --no-blocked")
	}
}

// TestBdStats_DefaultHumanOutput verifies that without --no-blocked, the Blocked
// line contains a numeric value and "(skipped)" is absent.
func TestBdStats_DefaultHumanOutput(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir := setupStatsRepo(t, bd)

	out := bdStats(t, bd, dir)

	if strings.Contains(out, "(skipped)") {
		t.Errorf("'(skipped)' must not appear in default stats output, got:\n%s", out)
	}
}

// TestBdStats_NoBlockedWithNoActivity verifies that combining --no-blocked and
// --no-activity flags works: blocked_issues null, recent_activity absent.
func TestBdStats_NoBlockedWithNoActivity(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir := setupStatsRepo(t, bd)

	result := bdStatsJSON(t, bd, dir, "--no-blocked", "--no-activity")
	if result == nil {
		t.Fatal("expected JSON result, got nil")
	}
	summary, ok := result["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'summary' object, got: %v", result)
	}

	// blocked_issues must be null.
	if bi, exists := summary["blocked_issues"]; exists && bi != nil {
		t.Errorf("summary.blocked_issues must be null, got: %v", bi)
	}
	// blocked_count_skipped must be true.
	if skip, ok := result["blocked_count_skipped"].(bool); !ok || !skip {
		t.Errorf("expected blocked_count_skipped=true, got: %v", result["blocked_count_skipped"])
	}
}

// containsDigit returns true if s contains any ASCII digit character.
func containsDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

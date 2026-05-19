//go:build cgo

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestUpdateStatusClosedRefusesOnAssigneeMismatch verifies that bd update --status=closed
// fails when the actor does not match the issue's assignee, mirroring bd close behavior.
func TestUpdateStatusClosedRefusesOnAssigneeMismatch(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ucm")

	// Create and claim as actor-a.
	issue := bdCreate(t, bd, dir, "Authority guard test", "--type", "task")
	bdUpdate(t, bd, dir, issue.ID, "--claim", "--actor", "actor-a")

	// Re-claim as actor-b (issue.Assignee is now "actor-b").
	bdUpdate(t, bd, dir, issue.ID, "--claim", "--actor", "actor-b")

	// actor-a tries to close — should fail because assignee is actor-b.
	out := bdUpdateFail(t, bd, dir, issue.ID, "--status", "closed", "--actor", "actor-a")

	// Error message must contain the authority context.
	if !strings.Contains(out, "assignee is") {
		t.Errorf("expected 'assignee is' in output, got: %q", out)
	}
	if !strings.Contains(out, "actor is") {
		t.Errorf("expected 'actor is' in output, got: %q", out)
	}
	if !strings.Contains(out, "reclaim or use --force") {
		t.Errorf("expected 'reclaim or use --force' in output, got: %q", out)
	}

	// Issue must remain in_progress (not closed).
	got := bdShow(t, bd, dir, issue.ID)
	if got.Status == types.StatusClosed {
		t.Error("issue must remain open after failed authority guard, but it was closed")
	}
}

// TestUpdateStatusClosedAllowsForce verifies that bd update --status=closed --force
// succeeds even when the actor does not match the assignee.
func TestUpdateStatusClosedAllowsForce(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ucf")

	issue := bdCreate(t, bd, dir, "Force close test", "--type", "task")
	bdUpdate(t, bd, dir, issue.ID, "--claim", "--actor", "actor-a")
	bdUpdate(t, bd, dir, issue.ID, "--claim", "--actor", "actor-b")

	// actor-a closes with --force — should succeed despite assignee mismatch.
	bdUpdate(t, bd, dir, issue.ID, "--status", "closed", "--actor", "actor-a", "--force")

	got := bdShow(t, bd, dir, issue.ID)
	if got.Status != types.StatusClosed {
		t.Errorf("expected closed after --force, got %s", got.Status)
	}
}

// TestUpdateStatusClosedAllowsMatchingAssignee verifies that bd update --status=closed
// succeeds when the actor matches the issue's assignee.
func TestUpdateStatusClosedAllowsMatchingAssignee(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ucm2")

	issue := bdCreate(t, bd, dir, "Matching actor test", "--type", "task")
	bdUpdate(t, bd, dir, issue.ID, "--claim", "--actor", "actor-a")

	// actor-a closes (matches the assignee) — should succeed.
	bdUpdate(t, bd, dir, issue.ID, "--status", "closed", "--actor", "actor-a")

	got := bdShow(t, bd, dir, issue.ID)
	if got.Status != types.StatusClosed {
		t.Errorf("expected closed when actor matches assignee, got %s", got.Status)
	}
}

// TestUpdateStatusClosedAllowsUnassigned verifies that bd update --status=closed
// succeeds when the issue has no assignee (anyone may close).
func TestUpdateStatusClosedAllowsUnassigned(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ucu")

	// Create issue without claiming (no assignee).
	issue := bdCreate(t, bd, dir, "Unassigned close test", "--type", "task")

	// Any actor may close an unassigned issue.
	bdUpdate(t, bd, dir, issue.ID, "--status", "closed", "--actor", "anyone")

	got := bdShow(t, bd, dir, issue.ID)
	if got.Status != types.StatusClosed {
		t.Errorf("expected closed for unassigned issue, got %s", got.Status)
	}
}

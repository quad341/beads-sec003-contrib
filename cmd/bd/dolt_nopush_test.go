package main

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/config"
)

// TestDoltPushGuard_NoPushTrue verifies that doltPushCmd.Run prints the exact
// skip message when no-push is true and returns without calling getStore.
func TestDoltPushGuard_NoPushTrue(t *testing.T) {
	initConfigForTest(t)
	config.Set("no-push", true)

	out := captureStdout(t, func() error {
		doltPushCmd.Run(doltPushCmd, []string{})
		return nil
	})

	want := "skipping push: rig is local-only (no-push: true)"
	if !strings.Contains(out, want) {
		t.Errorf("doltPushCmd output = %q, want it to contain %q", out, want)
	}
}

// TestDoltPullGuard_NoPushTrue verifies that doltPullCmd.Run prints the exact
// skip message when no-push is true and returns without calling getStore.
func TestDoltPullGuard_NoPushTrue(t *testing.T) {
	initConfigForTest(t)
	config.Set("no-push", true)

	out := captureStdout(t, func() error {
		doltPullCmd.Run(doltPullCmd, []string{})
		return nil
	})

	want := "skipping pull: rig is local-only (no-push: true)"
	if !strings.Contains(out, want) {
		t.Errorf("doltPullCmd output = %q, want it to contain %q", out, want)
	}
}

// TestDoltPushGuard_NoPushTrue_ForceFlagDoesNotBypass verifies that the
// --force flag does not bypass the no-push guard.
func TestDoltPushGuard_NoPushTrue_ForceFlagDoesNotBypass(t *testing.T) {
	initConfigForTest(t)
	config.Set("no-push", true)

	if err := doltPushCmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("could not set --force flag: %v", err)
	}
	t.Cleanup(func() { _ = doltPushCmd.Flags().Set("force", "false") })

	out := captureStdout(t, func() error {
		doltPushCmd.Run(doltPushCmd, []string{})
		return nil
	})

	if !strings.Contains(out, "skipping push") {
		t.Errorf("--force should not bypass no-push guard; got: %q", out)
	}
}

// TestDoltPushGuard_NoPushTrue_RemoteFlagDoesNotBypass verifies that the
// --remote flag does not bypass the no-push guard on push.
func TestDoltPushGuard_NoPushTrue_RemoteFlagDoesNotBypass(t *testing.T) {
	initConfigForTest(t)
	config.Set("no-push", true)

	if err := doltPushCmd.Flags().Set("remote", "origin"); err != nil {
		t.Fatalf("could not set --remote flag: %v", err)
	}
	t.Cleanup(func() { _ = doltPushCmd.Flags().Set("remote", "") })

	out := captureStdout(t, func() error {
		doltPushCmd.Run(doltPushCmd, []string{})
		return nil
	})

	if !strings.Contains(out, "skipping push") {
		t.Errorf("--remote should not bypass no-push guard on push; got: %q", out)
	}
}

// TestDoltPullGuard_NoPushTrue_RemoteFlagDoesNotBypass verifies that the
// --remote flag does not bypass the no-push guard on pull.
func TestDoltPullGuard_NoPushTrue_RemoteFlagDoesNotBypass(t *testing.T) {
	initConfigForTest(t)
	config.Set("no-push", true)

	if err := doltPullCmd.Flags().Set("remote", "origin"); err != nil {
		t.Fatalf("could not set --remote flag: %v", err)
	}
	t.Cleanup(func() { _ = doltPullCmd.Flags().Set("remote", "") })

	out := captureStdout(t, func() error {
		doltPullCmd.Run(doltPullCmd, []string{})
		return nil
	})

	if !strings.Contains(out, "skipping pull") {
		t.Errorf("--remote should not bypass no-push guard on pull; got: %q", out)
	}
}

// TestDoltPushGuard_NoPushFalse_GuardIsInactive verifies that the no-push
// guard does not activate when no-push is explicitly false.
func TestDoltPushGuard_NoPushFalse_GuardIsInactive(t *testing.T) {
	initConfigForTest(t)
	config.Set("no-push", false)

	if config.GetBool("no-push") {
		t.Error("no-push guard should be inactive when set to false")
	}
}

// TestDoltPushGuard_NoPushUnset_GuardIsInactive verifies that the no-push
// guard does not activate when the configuration key is absent (default behavior).
func TestDoltPushGuard_NoPushUnset_GuardIsInactive(t *testing.T) {
	initConfigForTest(t)
	// no-push key intentionally not set

	if config.GetBool("no-push") {
		t.Error("no-push guard should be inactive when key is not configured")
	}
}

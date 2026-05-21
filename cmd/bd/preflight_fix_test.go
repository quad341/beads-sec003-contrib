package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- fixVersionSync tests ---

// setupVersionSyncDir creates a temp dir with a git-like structure:
// cmd/bd/version.go and optionally a default.nix.
// Changes CWD to the dir and registers cleanup.
func setupVersionSyncDir(t *testing.T, versionGoContent, defaultNixContent string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	if versionGoContent != "" {
		vgoDir := filepath.Join(dir, "cmd", "bd")
		if err := os.MkdirAll(vgoDir, 0o755); err != nil {
			t.Fatalf("mkdir cmd/bd: %v", err)
		}
		if err := os.WriteFile(filepath.Join(vgoDir, "version.go"), []byte(versionGoContent), 0o644); err != nil {
			t.Fatalf("write version.go: %v", err)
		}
	}
	if defaultNixContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "default.nix"), []byte(defaultNixContent), 0o644); err != nil {
			t.Fatalf("write default.nix: %v", err)
		}
	}
}

// TestFixVersionSync_VersionGoNotFound verifies that fixVersionSync returns error
// when cmd/bd/version.go doesn't exist.
func TestFixVersionSync_VersionGoNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, _, err := fixVersionSync()
	if err == nil {
		t.Fatal("expected error when version.go not found, got nil")
	}
	if !strings.Contains(err.Error(), "version.go") {
		t.Errorf("expected error to mention version.go, got: %v", err)
	}
}

// TestFixVersionSync_UnparsableVersion verifies error when version constant can't be parsed.
func TestFixVersionSync_UnparsableVersion(t *testing.T) {
	setupVersionSyncDir(t, `package main
// no Version constant here
const Other = "1.0.0"
`, "")

	_, _, _, err := fixVersionSync()
	if err == nil {
		t.Fatal("expected error when Version constant not parseable, got nil")
	}
}

// TestFixVersionSync_NoDefaultNix verifies that missing default.nix → return false (skip).
func TestFixVersionSync_NoDefaultNix(t *testing.T) {
	setupVersionSyncDir(t, `package main
const Version = "1.0.0"
`, "")
	// No default.nix file.

	fixed, _, _, err := fixVersionSync()
	if err != nil {
		t.Fatalf("expected nil error when default.nix absent, got: %v", err)
	}
	if fixed {
		t.Error("expected fixed=false when default.nix absent (nothing to sync against)")
	}
}

// TestFixVersionSync_UnparsableNixVersion verifies error when version not found in default.nix.
func TestFixVersionSync_UnparsableNixVersion(t *testing.T) {
	setupVersionSyncDir(t, `package main
const Version = "1.0.0"
`, `{ # default.nix without a version field
  name = "beads";
}
`)

	_, _, _, err := fixVersionSync()
	if err == nil {
		t.Fatal("expected error when default.nix has no version field, got nil")
	}
}

// TestFixVersionSync_VersionsMatch verifies return false (nothing to fix) when in sync.
func TestFixVersionSync_VersionsMatch(t *testing.T) {
	setupVersionSyncDir(t, `package main
const Version = "1.2.3"
`, `{
  version = "1.2.3";
}
`)

	fixed, old, new, err := fixVersionSync()
	if err != nil {
		t.Fatalf("expected nil error on matching versions, got: %v", err)
	}
	if fixed {
		t.Errorf("expected fixed=false when versions match, got true (old=%q new=%q)", old, new)
	}
}

// TestFixVersionSync_VersionsDiffer verifies that fixVersionSync updates version.go
// when default.nix has a newer version.
func TestFixVersionSync_VersionsDiffer(t *testing.T) {
	setupVersionSyncDir(t, `package main
const Version = "1.0.0"
`, `{
  version = "1.2.3";
}
`)

	fixed, oldV, newV, err := fixVersionSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fixed {
		t.Errorf("expected fixed=true when versions differ, got false (old=%q new=%q)", oldV, newV)
	}
	if oldV != "1.0.0" {
		t.Errorf("oldVersion = %q, want %q", oldV, "1.0.0")
	}
	if newV != "1.2.3" {
		t.Errorf("newVersion = %q, want %q", newV, "1.2.3")
	}

	// Verify version.go was actually updated.
	content, err := os.ReadFile("cmd/bd/version.go")
	if err != nil {
		t.Fatalf("read updated version.go: %v", err)
	}
	if !strings.Contains(string(content), `"1.2.3"`) {
		t.Errorf("version.go not updated to 1.2.3:\n%s", content)
	}
}

// --- fixNixHash tests ---

// setupNixHashDir creates a temp dir with a git repo and go.sum, optionally with
// uncommitted go.sum changes, and writes default.nix with optional vendorHash.
// CWD is set to the temp dir.
func setupNixHashDir(t *testing.T, withGoSumChanges bool, defaultNixContent string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	// Init git repo.
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create and commit a base go.sum.
	gosum := filepath.Join(dir, "go.sum")
	if err := os.WriteFile(gosum, []byte("initial content\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}
	for _, args := range [][]string{
		{"add", "go.sum"},
		{"commit", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Run() //nolint:errcheck
	}

	if withGoSumChanges {
		// Modify go.sum without committing to simulate uncommitted changes.
		if err := os.WriteFile(gosum, []byte("modified content\n"), 0o644); err != nil {
			t.Fatalf("modify go.sum: %v", err)
		}
	}

	if defaultNixContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "default.nix"), []byte(defaultNixContent), 0o644); err != nil {
			t.Fatalf("write default.nix: %v", err)
		}
	}
}

// writeFakeNix creates a fake `nix` script in a temp bin dir and prepends it to PATH.
// The script simply outputs the given content and exits with the given code.
// Returns a cleanup function.
func writeFakeNix(t *testing.T, outputContent string, exitCode int) {
	t.Helper()
	binDir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\nexit %d\n", outputContent, exitCode)
	nixPath := filepath.Join(binDir, "nix")
	if err := os.WriteFile(nixPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake nix: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestFixNixHash_GoSumUnchanged verifies that fixNixHash returns false (skip) when
// go.sum has no uncommitted changes.
func TestFixNixHash_GoSumUnchanged(t *testing.T) {
	setupNixHashDir(t, false /* no changes */, "")

	fixed, _, _, err := fixNixHash()
	if err != nil {
		t.Fatalf("unexpected error when go.sum unchanged: %v", err)
	}
	if fixed {
		t.Error("expected fixed=false when go.sum has no uncommitted changes")
	}
}

// TestFixNixHash_NixNotInPath verifies that fixNixHash returns an error
// when nix is not in PATH (after detecting go.sum changes).
func TestFixNixHash_NixNotInPath(t *testing.T) {
	setupNixHashDir(t, true /* with changes */, `{
  vendorHash = "sha256-oldhashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
}
`)
	// Build a PATH that has git but NOT nix.
	// We find git's directory and set PATH to only that, ensuring nix is absent.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found — cannot construct nix-free PATH")
	}
	pathWithGitOnly := filepath.Dir(gitPath)
	t.Setenv("PATH", pathWithGitOnly)

	_, _, _, fixErr := fixNixHash()
	if fixErr == nil {
		t.Fatal("expected error when nix not in PATH, got nil")
	}
	if !strings.Contains(strings.ToLower(fixErr.Error()), "nix") {
		t.Errorf("expected error to mention 'nix', got: %v", fixErr)
	}
}

// TestFixNixHash_DefaultNixNotFound verifies that fixNixHash returns an error
// when default.nix doesn't exist.
func TestFixNixHash_DefaultNixNotFound(t *testing.T) {
	setupNixHashDir(t, true /* with changes */, "" /* no default.nix */)
	writeFakeNix(t, "", 0)

	_, _, _, err := fixNixHash()
	if err == nil {
		t.Fatal("expected error when default.nix not found, got nil")
	}
}

// TestFixNixHash_VendorHashNotFound verifies error when vendorHash pattern is absent.
func TestFixNixHash_VendorHashNotFound(t *testing.T) {
	setupNixHashDir(t, true /* with changes */, `{
  name = "beads";
  # no vendorHash field
}
`)
	writeFakeNix(t, "", 0)

	_, _, _, err := fixNixHash()
	if err == nil {
		t.Fatal("expected error when vendorHash not in default.nix, got nil")
	}
	if !strings.Contains(err.Error(), "vendorHash") {
		t.Errorf("expected error to mention 'vendorHash', got: %v", err)
	}
}

// TestFixNixHash_NixOutputNoHash verifies error when nix output can't be parsed.
func TestFixNixHash_NixOutputNoHash(t *testing.T) {
	setupNixHashDir(t, true /* with changes */, `{
  vendorHash = "sha256-oldhashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
}
`)
	// Fake nix produces output with no parseable hash.
	writeFakeNix(t, "error: no such thing happened", 1)

	_, _, _, err := fixNixHash()
	if err == nil {
		t.Fatal("expected error when nix output has no parseable hash, got nil")
	}
}

// TestFixNixHash_HashAlreadyCurrent verifies return false when nix reports
// the same hash as already in default.nix.
func TestFixNixHash_HashAlreadyCurrent(t *testing.T) {
	existingHash := "sha256-currenthashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	setupNixHashDir(t, true /* with changes */, fmt.Sprintf(`{
  vendorHash = "%s";
}
`, existingHash))
	// Fake nix outputs the same hash as the one already in default.nix.
	writeFakeNix(t, "got:    "+existingHash+"\n", 1)

	fixed, old, _, err := fixNixHash()
	if err != nil {
		t.Fatalf("unexpected error when hash already current: %v", err)
	}
	if fixed {
		t.Errorf("expected fixed=false when hash already matches, old=%q", old)
	}
}

// TestFixNixHash_HashDiffers verifies that fixNixHash updates default.nix
// when nix reports a different (correct) hash.
func TestFixNixHash_HashDiffers(t *testing.T) {
	oldHash := "sha256-oldhashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	newHash := "sha256-newhashBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="
	setupNixHashDir(t, true /* with changes */, fmt.Sprintf(`{
  vendorHash = "%s";
}
`, oldHash))
	// Fake nix outputs the new hash.
	writeFakeNix(t, "got:    "+newHash+"\n", 1)

	fixed, gotOld, gotNew, err := fixNixHash()
	if err != nil {
		t.Fatalf("unexpected error when hash differs: %v", err)
	}
	if !fixed {
		t.Errorf("expected fixed=true when hash differs, got false (old=%q new=%q)", gotOld, gotNew)
	}
	if gotOld != oldHash {
		t.Errorf("oldHash = %q, want %q", gotOld, oldHash)
	}
	if gotNew != newHash {
		t.Errorf("newHash = %q, want %q", gotNew, newHash)
	}

	// Verify default.nix was actually updated.
	content, err := os.ReadFile("default.nix")
	if err != nil {
		t.Fatalf("read default.nix: %v", err)
	}
	if !strings.Contains(string(content), newHash) {
		t.Errorf("default.nix not updated with newHash:\n%s", content)
	}
	if strings.Contains(string(content), oldHash) {
		t.Errorf("default.nix still contains oldHash after update:\n%s", content)
	}
}

// --- runFixes tests ---

// TestRunFixes_BothSkipped verifies runFixes exits 0 when nothing needs fixing.
// We run it from a clean temp dir where both fixNixHash() (no go.sum changes)
// and fixVersionSync() (no version.go) result in skip/error that doesn't set hasError.
func TestRunFixes_BothSkipped(t *testing.T) {
	// Set up a git repo with committed go.sum (no uncommitted changes).
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Run() //nolint:errcheck
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("sum\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}
	exec.Command("git", "-C", dir, "add", "go.sum").Run()     //nolint:errcheck
	exec.Command("git", "-C", dir, "commit", "-m", "i").Run() //nolint:errcheck

	// No version.go or default.nix present.
	// fixNixHash → skip (go.sum unchanged).
	// fixVersionSync → error (no version.go) but we expect runFixes to exit 1 on error.
	// So let's also create a version.go with matching default.nix so both skip cleanly.
	vgoDir := filepath.Join(dir, "cmd", "bd")
	os.MkdirAll(vgoDir, 0o755) //nolint:errcheck
	os.WriteFile(filepath.Join(vgoDir, "version.go"), []byte(`package main
const Version = "1.0.0"
`), 0o644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "default.nix"), []byte(`{
  version = "1.0.0";
  vendorHash = "sha256-aaaa=";
}
`), 0o644) //nolint:errcheck

	// runFixes calls os.Exit internally — can't call directly.
	// Instead, verify fixNixHash and fixVersionSync both return non-error skips.
	fixed1, _, _, err1 := fixNixHash()
	fixed2, _, _, err2 := fixVersionSync()
	if err1 != nil {
		t.Errorf("fixNixHash error (should skip): %v", err1)
	}
	if fixed1 {
		t.Error("fixNixHash: expected skip, got fixed=true")
	}
	if err2 != nil {
		t.Errorf("fixVersionSync error (should skip): %v", err2)
	}
	if fixed2 {
		t.Error("fixVersionSync: expected skip (versions match), got fixed=true")
	}
}

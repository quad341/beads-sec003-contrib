package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BuildIntegration resolves ref to an exact commit SHA in repoDir, builds
// pkgPath from that commit's tree into outBin, and returns the resolved SHA.
// The build happens in a temporary git worktree outside repoDir so the
// caller's own working tree and HEAD are never touched; the worktree is
// removed again before returning.
func BuildIntegration(ctx context.Context, repoDir, ref, pkgPath, outBin string) (string, error) {
	sha, err := runGit(ctx, repoDir, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("build integration: resolve ref %q: %w", ref, err)
	}

	worktreeDir, err := os.MkdirTemp("", "driver-core-integration-*")
	if err != nil {
		return "", fmt.Errorf("build integration: create worktree dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(worktreeDir) }()

	if _, err := runGit(ctx, repoDir, "worktree", "add", "--detach", worktreeDir, sha); err != nil {
		return "", fmt.Errorf("build integration: add worktree for %s: %w", sha, err)
	}
	defer func() { _, _ = runGit(ctx, repoDir, "worktree", "remove", "--force", worktreeDir) }()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", outBin, pkgPath)
	cmd.Dir = worktreeDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build integration: go build %s at %s: %w\n%s", pkgPath, sha, err, out)
	}

	return sha, nil
}

// runGit runs a git subcommand in dir and returns its trimmed stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

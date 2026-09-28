package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newTestGitRepo creates a throwaway git repository with two commits of a
// trivial `main` package, so BuildIntegration's tests never need to build
// the real (slow) cmd/bd and never risk touching this worktree's own state.
func newTestGitRepo(t *testing.T) (dir string, firstSHA, secondSHA string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=driver-core-test", "GIT_AUTHOR_EMAIL=driver-core-test@example.com",
			"GIT_COMMITTER_NAME=driver-core-test", "GIT_COMMITTER_EMAIL=driver-core-test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module driver-core-fixture\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	writeMain := func(version string) {
		src := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"" + version + "\") }\n"
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
			t.Fatalf("WriteFile main.go: %v", err)
		}
	}

	writeMain("v1")
	run("add", ".")
	run("commit", "-m", "v1")
	firstSHA = run("rev-parse", "HEAD")

	writeMain("v2")
	run("add", ".")
	run("commit", "-m", "v2")
	secondSHA = run("rev-parse", "HEAD")

	return dir, firstSHA, secondSHA
}

func TestBuildIntegration_ResolvesRefToExactSHA(t *testing.T) {
	repoDir, _, secondSHA := newTestGitRepo(t)
	outBin := filepath.Join(t.TempDir(), "fixture-bin")

	sha, err := BuildIntegration(context.Background(), repoDir, "HEAD", ".", outBin)
	if err != nil {
		t.Fatalf("BuildIntegration: %v", err)
	}
	if sha != secondSHA {
		t.Errorf("BuildIntegration sha = %q, want HEAD = %q", sha, secondSHA)
	}
}

func TestBuildIntegration_ProducesBinaryMatchingTheBuiltRef(t *testing.T) {
	repoDir, firstSHA, _ := newTestGitRepo(t)
	outBin := filepath.Join(t.TempDir(), "fixture-bin")

	sha, err := BuildIntegration(context.Background(), repoDir, firstSHA, ".", outBin)
	if err != nil {
		t.Fatalf("BuildIntegration: %v", err)
	}
	if sha != firstSHA {
		t.Errorf("BuildIntegration sha = %q, want %q", sha, firstSHA)
	}

	out, err := exec.Command(outBin).Output()
	if err != nil {
		t.Fatalf("running built binary: %v", err)
	}
	if strings.TrimSpace(string(out)) != "v1" {
		t.Errorf("built binary output = %q, want v1 (built from firstSHA, not the newer working tree content)", out)
	}
}

func TestBuildIntegration_NeverTouchesCallersWorkingTree(t *testing.T) {
	repoDir, firstSHA, secondSHA := newTestGitRepo(t)
	outBin := filepath.Join(t.TempDir(), "fixture-bin")

	untracked := filepath.Join(repoDir, "untracked-scratch.txt")
	if err := os.WriteFile(untracked, []byte("do not touch"), 0o644); err != nil {
		t.Fatalf("WriteFile untracked: %v", err)
	}

	statusBefore := gitStatusPorcelain(t, repoDir)

	if _, err := BuildIntegration(context.Background(), repoDir, firstSHA, ".", outBin); err != nil {
		t.Fatalf("BuildIntegration: %v", err)
	}

	statusAfter := gitStatusPorcelain(t, repoDir)
	if statusBefore != statusAfter {
		t.Errorf("BuildIntegration disturbed the caller's working tree: before=%q after=%q", statusBefore, statusAfter)
	}
	data, err := os.ReadFile(untracked)
	if err != nil {
		t.Fatalf("reading untracked file after BuildIntegration: %v", err)
	}
	if string(data) != "do not touch" {
		t.Errorf("untracked file content changed: %q", data)
	}

	headAfter := gitRevParse(t, repoDir, "HEAD")
	if headAfter != secondSHA {
		t.Errorf("BuildIntegration moved the caller's HEAD: got %q, want %q (unchanged)", headAfter, secondSHA)
	}
}

func gitStatusPorcelain(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain: %v", err)
	}
	return string(out)
}

func gitRevParse(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

func TestBuildIntegration_CleansUpTemporaryWorktree(t *testing.T) {
	repoDir, firstSHA, _ := newTestGitRepo(t)
	outBin := filepath.Join(t.TempDir(), "fixture-bin")

	if _, err := BuildIntegration(context.Background(), repoDir, firstSHA, ".", outBin); err != nil {
		t.Fatalf("BuildIntegration: %v", err)
	}

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	worktreeCount := strings.Count(string(out), "worktree ")
	if worktreeCount != 1 {
		t.Errorf("expected exactly 1 worktree (the main one) after BuildIntegration cleans up, got %d:\n%s", worktreeCount, out)
	}
}

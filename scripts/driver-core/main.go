package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	integrationRef := flag.String("integration-ref", "HEAD", "git ref of the integration build under test")
	integrationRepo := flag.String("integration-repo", "", "path to the beads repo checkout to build the integration binary from (required)")
	oracleDataDir := flag.String("oracle-data-dir", "", "dolt data directory containing the historical corpus to replay (required)")
	workDir := flag.String("work-dir", "", "bd project directory to replay mutations into; created and initialized if it doesn't already exist (required)")
	outDir := flag.String("out-dir", "", "directory to write replay_runs/commit_replay_results/mismatches/metric_samples JSONL files to (required)")
	sampleSize := flag.Int("sample-size", 0, "number of evenly-spaced commits to sample; 0 replays the full history exhaustively")
	flag.Parse()

	if *integrationRepo == "" || *oracleDataDir == "" || *workDir == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "usage: driver-core --integration-repo DIR --oracle-data-dir DIR --work-dir DIR --out-dir DIR [--integration-ref REF] [--sample-size N]")
		os.Exit(2)
	}

	if err := run(*integrationRef, *integrationRepo, *oracleDataDir, *workDir, *outDir, *sampleSize); err != nil {
		log.Fatalf("driver-core: %v", err)
	}
}

func run(integrationRef, integrationRepo, oracleDataDir, workDir, outDir string, sampleSize int) error {
	ctx := context.Background()

	toolsDir, err := os.MkdirTemp("", "driver-core-tools-*")
	if err != nil {
		return fmt.Errorf("creating tools dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(toolsDir) }()

	oracleQueryBin := filepath.Join(toolsDir, "oracle-query")
	mutationTranslatorBin := filepath.Join(toolsDir, "mutation-translator")
	integrationBin := filepath.Join(toolsDir, "integration-bd")

	if err := goBuild(ctx, integrationRepo, "./scripts/oracle-query", oracleQueryBin); err != nil {
		return fmt.Errorf("building oracle-query: %w", err)
	}
	if err := goBuild(ctx, integrationRepo, "./scripts/mutation-translator", mutationTranslatorBin); err != nil {
		return fmt.Errorf("building mutation-translator: %w", err)
	}
	if _, err := BuildIntegration(ctx, integrationRepo, integrationRef, "./cmd/bd", integrationBin); err != nil {
		return fmt.Errorf("building integration binary: %w", err)
	}

	if err := ensureWorkProject(ctx, workDir, integrationBin); err != nil {
		return fmt.Errorf("preparing work project: %w", err)
	}
	workDataDir, err := findEmbeddedDoltDir(workDir)
	if err != nil {
		return fmt.Errorf("locating work project's data dir: %w", err)
	}

	replayRun, err := Run(ctx, RunConfig{
		IntegrationRef:  integrationRef,
		IntegrationRepo: integrationRepo,
		OracleDataDir:   oracleDataDir,
		WorkDir:         workDir,
		WorkDataDir:     workDataDir,
		OutDir:          outDir,
		SampleSize:      sampleSize,
		Tools: Tools{
			OracleQueryBin:        oracleQueryBin,
			MutationTranslatorBin: mutationTranslatorBin,
			IntegrationBin:        integrationBin,
		},
	})
	if err != nil {
		return err
	}

	fmt.Printf("replay run %s: status=%s mode=%s integration_sha=%s\n", replayRun.ID, replayRun.Status, replayRun.Mode, replayRun.IntegrationSHA)
	return nil
}

// goBuild builds pkgPath (relative to dir) into outBin using whatever is
// currently checked out in dir -- unlike BuildIntegration, it does no ref
// resolution or worktree isolation, because the harness's own tools
// (oracle-query, mutation-translator) are always built from the checkout
// driver-core itself runs from, never from a pinned historical ref.
func goBuild(ctx context.Context, dir, pkgPath, outBin string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-o", outBin, pkgPath)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s in %s: %w\n%s", pkgPath, dir, err, out)
	}
	return nil
}

// ensureWorkProject initializes workDir as a bd project via integrationBin
// if it isn't one already, so a caller can point driver-core at a fresh
// empty directory without a separate manual bootstrap step.
func ensureWorkProject(ctx context.Context, workDir, integrationBin string) error {
	if _, err := os.Stat(filepath.Join(workDir, ".beads")); err == nil {
		return nil
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("ensure work project: %w", err)
	}
	cmd := exec.CommandContext(ctx, integrationBin, "init", "--non-interactive", "--role=maintainer")
	cmd.Dir = workDir
	cmd.Env = sanitizedEnv(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ensure work project: init: %w\n%s", err, out)
	}
	return nil
}

// findEmbeddedDoltDir returns the embedded Dolt data directory bd init
// created under dir. Mirrors fixture_test.go's dataDir test helper: Glob's
// "*" also matches a sibling ".lock" file, so matches must be filtered to
// directories.
func findEmbeddedDoltDir(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, ".beads", "embeddeddolt", "*"))
	if err != nil {
		return "", fmt.Errorf("find embedded dolt dir under %s: %w", dir, err)
	}
	for _, m := range matches {
		if info, statErr := os.Stat(m); statErr == nil && info.IsDir() {
			return m, nil
		}
	}
	return "", fmt.Errorf("find embedded dolt dir under %s: no directory among matches=%v", dir, matches)
}

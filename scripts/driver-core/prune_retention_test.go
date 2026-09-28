package main

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDriverCore_PruneRetention is be-hlte6 (adopted from be-hs42e.5 §10 item
// 2 / be-hs42e.5.5): bd prune interacts directly with retention, so this
// scenario runs bd prune against a working clone carrying versioned history
// the replay-with-oracle harness (be-hs42e.5.2) depends on, then checks --
// both with and without a follow-up bd flatten -- whether that history stays
// retrievable through the exact AS-OF path the harness uses (oracle-query,
// be-3j80a). A versioned-history feature quietly losing history to a prune
// interaction must come back as a visible failure, never a silent pass.
//
// Negative control: bd prune alone deletes only the CURRENT row (issues,
// deps, labels, events, comments for the matched closed bead) -- see
// cmd/bd/prune.go. It does not rewrite or collapse Dolt commit history, so
// an AS-OF query pinned to a commit that predates the prune must still
// resolve, byte-identical to what it returned before the prune ran.
//
// Positive control: prune's own help text names bd flatten as the natural
// follow-up ("For full Dolt storage reclaim ... follow with bd flatten"),
// and flatten's docs call itself "irreversible -- all commit history is
// lost" (cmd/bd/flatten.go). A pre-flatten commit ref must therefore become
// genuinely unresolvable afterward, and that has to surface as a real error
// from the harness's oracle-query subprocess, never as a silent empty or
// stale result.
func TestDriverCore_PruneRetention(t *testing.T) {
	requireBd(t)
	requireDolt(t)
	repoRoot := findRepoRoot(t)

	toolsDir := t.TempDir()
	oracleQueryBin := filepath.Join(toolsDir, "oracle-query")
	mutationTranslatorBin := filepath.Join(toolsDir, "mutation-translator")
	if err := goBuild(context.Background(), repoRoot, "./scripts/oracle-query", oracleQueryBin); err != nil {
		t.Fatalf("building oracle-query: %v", err)
	}
	if err := goBuild(context.Background(), repoRoot, "./scripts/mutation-translator", mutationTranslatorBin); err != nil {
		t.Fatalf("building mutation-translator: %v", err)
	}
	integrationBin := filepath.Join(toolsDir, "integration-bd")
	if _, err := BuildIntegration(context.Background(), repoRoot, "HEAD", "./cmd/bd", integrationBin); err != nil {
		t.Fatalf("BuildIntegration: %v", err)
	}
	tools := Tools{OracleQueryBin: oracleQueryBin, MutationTranslatorBin: mutationTranslatorBin, IntegrationBin: integrationBin}

	t.Run("negative_control_prune_alone_retains_history", func(t *testing.T) {
		workDir, workData, idA, preRef, want := setupPruneRetentionFixture(t, repoRoot, integrationBin, tools)

		runBdBin(t, integrationBin, workDir, "prune", "--pattern", idA, "--force")

		got, err := QueryOracleSubprocess(context.Background(), oracleQueryBin, workData, preRef, idA)
		if err != nil {
			t.Fatalf("AS-OF query for %s at pre-prune commit %s failed after a bare prune (history should be untouched): %v", idA, preRef, err)
		}
		if got == nil {
			t.Fatalf("AS-OF query for %s at pre-prune commit %s returned no row after a bare prune (history should be untouched)", idA, preRef)
		}
		if got["title"] != want["title"] {
			t.Errorf("AS-OF row for %s at %s after prune = %+v, want title %q (prune must not alter retained history)", idA, preRef, got, want["title"])
		}
	})

	t.Run("positive_control_flatten_after_prune_loses_history", func(t *testing.T) {
		workDir, workData, idA, preRef, _ := setupPruneRetentionFixture(t, repoRoot, integrationBin, tools)

		runBdBin(t, integrationBin, workDir, "prune", "--pattern", idA, "--force")
		runBdBin(t, integrationBin, workDir, "flatten", "--force")

		_, err := QueryOracleSubprocess(context.Background(), oracleQueryBin, workData, preRef, idA)
		if err == nil {
			t.Fatalf("AS-OF query for %s at pre-flatten commit %s unexpectedly succeeded after bd flatten -- flatten claims to make all prior history unresolvable, so a silent success here means a real, versioned-history-eating divergence was NOT surfaced", idA, preRef)
		}
	})
}

// setupPruneRetentionFixture builds a small oracle history (create + close,
// so the bead is both historically interesting and immediately prunable),
// replays it into a fresh work project via the full driver-core Run loop,
// and returns the work project plus the pre-prune commit ref and the row it
// must keep producing at that ref. Each subtest gets its own fixture so
// prune/flatten in one can never contaminate the other.
func setupPruneRetentionFixture(t *testing.T, repoRoot, integrationBin string, tools Tools) (workDir, workData, idA, preRef string, want map[string]string) {
	t.Helper()

	oracleDir := initBdProjectWith(t, "oracle", integrationBin)
	oracleData := dataDir(t, oracleDir)

	out := runBdBin(t, integrationBin, oracleDir, "create", "Retention fixture bead", "--type", "task", "--json")
	idA = jsonID(t, out)
	runBdBin(t, integrationBin, oracleDir, "close", idA, "--reason", "done")

	workDir = initBdProjectWith(t, "work", integrationBin)
	workData = dataDir(t, workDir)

	outDir := t.TempDir()
	run, err := Run(context.Background(), RunConfig{
		IntegrationRef:  "HEAD",
		IntegrationRepo: repoRoot,
		OracleDataDir:   oracleData,
		WorkDir:         workDir,
		WorkDataDir:     workData,
		OutDir:          outDir,
		Tools:           tools,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("run.Status = %q, want completed", run.Status)
	}

	preRef, err = headCommitOf(context.Background(), workData)
	if err != nil {
		t.Fatalf("headCommitOf(%s): %v", workData, err)
	}

	want, err = QueryOracleSubprocess(context.Background(), tools.OracleQueryBin, workData, preRef, idA)
	if err != nil {
		t.Fatalf("baseline AS-OF query for %s at %s: %v", idA, preRef, err)
	}
	if want == nil {
		t.Fatalf("baseline AS-OF query for %s at %s returned no row", idA, preRef)
	}
	return workDir, workData, idA, preRef, want
}

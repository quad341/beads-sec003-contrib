package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDriverCore_AF1UC2FullLoop is be-sodi8 AC8: a full AF1+UC2 loop against
// a small local fixture clone with several synthetic historical commits
// spanning multiple mutation kinds (create, update, dep_add, close,
// dep_remove), asserting correct COMMIT_REPLAY_RESULT rows and that a
// deliberately-injected mismatch produces a correct MISMATCH row.
func TestDriverCore_AF1UC2FullLoop(t *testing.T) {
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
	sha, err := BuildIntegration(context.Background(), repoRoot, "HEAD", "./cmd/bd", integrationBin)
	if err != nil {
		t.Fatalf("BuildIntegration: %v", err)
	}
	if sha == "" {
		t.Fatalf("BuildIntegration returned empty sha")
	}

	// ---- oracle: synthetic historical commits ------------------------------
	oracleDir := initBdProject(t, "oracle")
	oracleData := dataDir(t, oracleDir)

	outA := runBd(t, oracleDir, "create", "Widget A", "--type", "task", "--json")
	idA := jsonID(t, outA)
	outB := runBd(t, oracleDir, "create", "Widget B", "--type", "task", "--json")
	idB := jsonID(t, outB)
	runBd(t, oracleDir, "update", idA, "--description", "updated desc")
	runBd(t, oracleDir, "dep", "add", idA, idB)
	runBd(t, oracleDir, "close", idA, "--reason", "done")
	runBd(t, oracleDir, "dep", "remove", idA, idB)

	// ---- work: fresh project driven by the integration build --------------
	workDir := initBdProjectWith(t, "work", integrationBin)
	workData := dataDir(t, workDir)

	outDir := t.TempDir()
	tools := Tools{OracleQueryBin: oracleQueryBin, MutationTranslatorBin: mutationTranslatorBin, IntegrationBin: integrationBin}

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
	if run.IntegrationSHA != sha {
		t.Fatalf("run.IntegrationSHA = %q, want %q", run.IntegrationSHA, sha)
	}
	if run.Mode != "exhaustive" {
		t.Fatalf("run.Mode = %q, want exhaustive", run.Mode)
	}

	results := readCommitReplayResults(t, outDir)
	if len(results) == 0 {
		t.Fatalf("no COMMIT_REPLAY_RESULT rows written")
	}
	kinds := map[string]bool{}
	for _, r := range results {
		if r.RunID != run.ID {
			t.Errorf("result run_id = %q, want %q", r.RunID, run.ID)
		}
		if !r.Matched {
			t.Errorf("unexpected mismatch on faithful replay: commit=%s issue=%s kind=%s", r.SourceCommit, r.IssueID, r.MutationKind)
		}
		kinds[r.MutationKind] = true
	}
	for _, want := range []string{"create", "update", "dep_add", "dep_remove"} {
		if !kinds[want] {
			t.Errorf("no COMMIT_REPLAY_RESULT row observed with mutation_kind=%q; got kinds=%v", want, kinds)
		}
	}

	// ---- deliberately injected mismatch ------------------------------------
	runDolt(t, workData, "sql", "-q", "UPDATE issues SET title='CORRUPTED' WHERE id='"+idB+"'")
	badRow, err := QueryOracleSubprocess(context.Background(), oracleQueryBin, workData, headCommit(t, workData), idB)
	if err != nil {
		t.Fatalf("querying corrupted row: %v", err)
	}
	goodRow, err := QueryOracleSubprocess(context.Background(), oracleQueryBin, oracleData, headCommit(t, oracleData), idB)
	if err != nil {
		t.Fatalf("querying oracle row: %v", err)
	}
	goodJSON, _ := json.Marshal(filterComparable(goodRow))
	badJSON, _ := json.Marshal(filterComparable(badRow))
	cmp, err := Compare(goodJSON, badJSON)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if cmp.Matched {
		t.Fatalf("expected mismatch after deliberate corruption of %s", idB)
	}

	store, err := NewStore(outDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.WriteMismatch(Mismatch{
		RunID: run.ID, SourceCommit: headCommit(t, workData), IssueID: idB,
		Category: cmp.Mismatch.Category, ExpectedJSON: cmp.Mismatch.ExpectedJSON, ActualJSON: cmp.Mismatch.ActualJSON,
	}); err != nil {
		t.Fatalf("WriteMismatch: %v", err)
	}

	mismatches := readMismatches(t, outDir)
	found := false
	for _, m := range mismatches {
		if m.IssueID == idB && m.RunID == run.ID {
			found = true
			var actual map[string]string
			if err := json.Unmarshal(m.ActualJSON, &actual); err != nil {
				t.Fatalf("mismatch ActualJSON not valid: %v", err)
			}
			if actual["title"] != "CORRUPTED" {
				t.Errorf("mismatch ActualJSON does not carry the corrupted value: %+v", actual)
			}
		}
	}
	if !found {
		t.Fatalf("no MISMATCH row found for deliberately-corrupted issue %s", idB)
	}

	metrics := readMetricSamples(t, outDir)
	sawStorage, sawLatency := false, false
	for _, m := range metrics {
		if m.RunID != run.ID {
			continue
		}
		switch m.Name {
		case "storage_bytes":
			sawStorage = true
		case "write_latency_ms":
			sawLatency = true
		}
	}
	if !sawStorage || !sawLatency {
		t.Errorf("expected both storage_bytes and write_latency_ms metric samples, got sawStorage=%v sawLatency=%v", sawStorage, sawLatency)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func readCommitReplayResults(t *testing.T, outDir string) []CommitReplayResult {
	t.Helper()
	return readJSONLTyped[CommitReplayResult](t, filepath.Join(outDir, "commit_replay_results.jsonl"))
}

func readMismatches(t *testing.T, outDir string) []Mismatch {
	t.Helper()
	return readJSONLTyped[Mismatch](t, filepath.Join(outDir, "mismatches.jsonl"))
}

func readMetricSamples(t *testing.T, outDir string) []MetricSample {
	t.Helper()
	return readJSONLTyped[MetricSample](t, filepath.Join(outDir, "metric_samples.jsonl"))
}

func readJSONLTyped[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading %s: %v", path, err)
	}
	var out []T
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("unmarshaling line from %s: %v (line: %s)", path, err, line)
		}
		out = append(out, v)
	}
	return out
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Tools pins the exact binaries a Run invocation replays through: the
// harness's own read-side helper (oracle-query, be-3j80a), its write-side
// translator (mutation-translator, be-2cp1d), and the integration build
// under test (NFR4).
type Tools struct {
	OracleQueryBin        string
	MutationTranslatorBin string
	IntegrationBin        string
}

// RunConfig is everything one Run invocation needs: which integration build
// to test (IntegrationRef/IntegrationRepo), which historical corpus is the
// oracle (OracleDataDir), which fresh project to replay mutations into
// (WorkDir/WorkDataDir), where to persist results (OutDir), and the tool
// binaries to shell out to. SampleSize <= 0 means UC2's exhaustive loop;
// SampleSize > 0 selects that many evenly-spaced commits (AF1 sampled mode).
type RunConfig struct {
	IntegrationRef  string
	IntegrationRepo string
	OracleDataDir   string
	WorkDir         string
	WorkDataDir     string
	OutDir          string
	SampleSize      int
	Tools           Tools
}

// Run resolves the integration build under test to an exact commit,
// discovers every issue touched by the oracle's commit history (or an
// evenly-spaced sample of it), and for each one replays the corresponding
// mutation into the work project and compares the result against the
// oracle, persisting a CommitReplayResult (and a Mismatch, when the
// comparison fails) plus storage/latency MetricSamples for every step.
// A mismatch does not abort the run -- AC2 requires a result row for every
// pair the run visits, matched or not; only an infrastructure failure
// (a broken ref, a crashed subprocess, a write error) does.
func Run(ctx context.Context, cfg RunConfig) (ReplayRun, error) {
	sha, err := runGit(ctx, cfg.IntegrationRepo, "rev-parse", cfg.IntegrationRef)
	if err != nil {
		return ReplayRun{}, fmt.Errorf("run: resolve integration ref %q: %w", cfg.IntegrationRef, err)
	}

	mode := "exhaustive"
	if cfg.SampleSize > 0 {
		mode = "sampled"
	}
	run := ReplayRun{
		ID:             GenerateRunID(),
		IntegrationRef: cfg.IntegrationRef,
		IntegrationSHA: sha,
		Mode:           mode,
		SampleSize:     cfg.SampleSize,
		StartedAt:      time.Now().UTC(),
		Status:         "running",
	}

	store, err := NewStore(cfg.OutDir)
	if err != nil {
		return run, fmt.Errorf("run: %w", err)
	}

	if err := runReplayLoop(ctx, cfg, run, store); err != nil {
		run.Status = "failed"
		run.FinishedAt = time.Now().UTC()
		if writeErr := store.WriteReplayRun(run); writeErr != nil {
			return run, fmt.Errorf("run: %w (and failed to record failed status: %v)", err, writeErr)
		}
		return run, fmt.Errorf("run: %w", err)
	}

	run.Status = "completed"
	run.FinishedAt = time.Now().UTC()
	if err := store.WriteReplayRun(run); err != nil {
		return run, fmt.Errorf("run: writing completed replay run: %w", err)
	}
	return run, nil
}

func runReplayLoop(ctx context.Context, cfg RunConfig, run ReplayRun, store *Store) error {
	commits, err := ListCommits(ctx, cfg.OracleDataDir)
	if err != nil {
		return err
	}
	commits = selectSample(commits, cfg.SampleSize)

	for i := 0; i+1 < len(commits); i++ {
		from, to := commits[i], commits[i+1]
		touched, err := DiscoverTouchedIssues(ctx, cfg.OracleDataDir, from.Hash, to.Hash)
		if err != nil {
			return err
		}
		for _, ti := range touched {
			if err := replayOne(ctx, cfg, run.ID, store, from.Hash, to.Hash, ti, to.IsMerge); err != nil {
				return err
			}
		}
	}
	return nil
}

// replayOne replays a single touched issue's mutation for one commit pair,
// records the comparison outcome, and emits its storage/latency samples.
func replayOne(ctx context.Context, cfg RunConfig, runID string, store *Store, from, to string, ti TouchedIssue, isMerge bool) error {
	mutationKind := MutationKindFor(ti.Diff, isMerge)

	var writeLatency time.Duration
	readOracle := func(ctx context.Context) (map[string]string, error) {
		return QueryOracleSubprocess(ctx, cfg.Tools.OracleQueryBin, cfg.OracleDataDir, to, ti.IssueID)
	}
	replay := func(ctx context.Context) (map[string]string, error) {
		start := time.Now()
		err := ReplayMutationSubprocess(ctx, cfg.Tools.MutationTranslatorBin, cfg.Tools.IntegrationBin, cfg.OracleDataDir, cfg.WorkDir, from, to, ti.IssueID)
		writeLatency = time.Since(start)
		if err != nil {
			return nil, err
		}
		workHead, err := headCommitOf(ctx, cfg.WorkDataDir)
		if err != nil {
			return nil, err
		}
		return QueryOracleSubprocess(ctx, cfg.Tools.OracleQueryBin, cfg.WorkDataDir, workHead, ti.IssueID)
	}

	result, err := ReplayAndCompare(ctx, readOracle, replay)
	if err != nil {
		return fmt.Errorf("replay %s at %s: %w", ti.IssueID, to, err)
	}

	if err := store.WriteCommitReplayResult(CommitReplayResult{
		RunID:         runID,
		SourceCommit:  to,
		IssueID:       ti.IssueID,
		MutationKind:  mutationKind,
		Matched:       result.Matched,
		OracleHash:    result.OracleHash,
		CandidateHash: result.CandidateHash,
	}); err != nil {
		return fmt.Errorf("write commit replay result for %s at %s: %w", ti.IssueID, to, err)
	}

	if !result.Matched && result.Mismatch != nil {
		if err := store.WriteMismatch(Mismatch{
			RunID:        runID,
			SourceCommit: to,
			IssueID:      ti.IssueID,
			Category:     result.Mismatch.Category,
			ExpectedJSON: result.Mismatch.ExpectedJSON,
			ActualJSON:   result.Mismatch.ActualJSON,
		}); err != nil {
			return fmt.Errorf("write mismatch for %s at %s: %w", ti.IssueID, to, err)
		}
	}

	if err := store.WriteMetricSample(MetricSample{
		RunID: runID, Name: "write_latency_ms", Value: float64(writeLatency.Milliseconds()), SampledAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("write latency metric for %s at %s: %w", ti.IssueID, to, err)
	}
	size, err := dirSize(cfg.WorkDataDir)
	if err != nil {
		return fmt.Errorf("measure work store size after %s at %s: %w", ti.IssueID, to, err)
	}
	if err := store.WriteMetricSample(MetricSample{
		RunID: runID, Name: "storage_bytes", Value: float64(size), SampledAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("write storage metric for %s at %s: %w", ti.IssueID, to, err)
	}

	return nil
}

// selectSample returns all of steps, copied, when n<=0 or n>=len(steps)
// (UC2's exhaustive loop); otherwise it returns exactly n elements as a
// deterministic, strictly-ascending-index subsequence of steps (AF1's
// evenly-spaced sampled mode) -- never reordered, never randomized.
func selectSample[T any](steps []T, n int) []T {
	if n <= 0 || n >= len(steps) {
		out := make([]T, len(steps))
		copy(out, steps)
		return out
	}
	if n == 1 {
		return []T{steps[0]}
	}
	last := len(steps) - 1
	out := make([]T, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, steps[i*last/(n-1)])
	}
	return out
}

// headCommitOf returns dataDir's current head commit hash. Mirrors the
// query fixture_test.go's headCommit test helper already relies on
// empirically (dolt_log ordered newest-first by commit_order) rather than
// gambling on whether Dolt's "AS OF" clause resolves the literal ref
// "HEAD" the way git does.
func headCommitOf(ctx context.Context, dataDir string) (string, error) {
	header, rows, err := doltQuery(ctx, dataDir, "SELECT commit_hash FROM dolt_log ORDER BY commit_order DESC LIMIT 1")
	if err != nil {
		return "", fmt.Errorf("head commit of %s: %w", dataDir, err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("head commit of %s: dolt_log returned no rows", dataDir)
	}
	row := rowMap(header, rows[0])
	return row["commit_hash"], nil
}

// QueryOracleSubprocess shells out to the oracle-query tool (be-3j80a) to
// read issueID's row as of ref in dataDir, tolerating the tool's literal
// "null" JSON output for "no such row existed at that ref".
func QueryOracleSubprocess(ctx context.Context, oracleQueryBin, dataDir, ref, issueID string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, oracleQueryBin, "-dir", dataDir, "-ref", ref, "-issue", issueID)
	cmd.Env = sanitizedEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("query oracle subprocess: %w: %s", err, ee.Stderr)
		}
		return nil, fmt.Errorf("query oracle subprocess: %w", err)
	}
	var row map[string]string
	if err := json.Unmarshal(out, &row); err != nil {
		return nil, fmt.Errorf("query oracle subprocess: parse output: %w", err)
	}
	return row, nil
}

// ReplayMutationSubprocess translates the mutation to issueID between from
// and to (as recorded in the oracle's history at dataDir) and applies it to
// workDir, executing bd commands through integrationBin rather than
// whatever "bd" happens to resolve to on the ambient PATH: mutation-
// translator's Execute always resolves "bd" via os/exec's own PATH lookup
// with no override flag, so pinning it means shimming PATH for this
// subprocess only.
func ReplayMutationSubprocess(ctx context.Context, mutationTranslatorBin, integrationBin, dataDir, workDir, from, to, issueID string) error {
	shimDir, cleanup, err := prependToPath(integrationBin)
	if err != nil {
		return fmt.Errorf("replay mutation subprocess: %w", err)
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, mutationTranslatorBin,
		"--data-dir", dataDir,
		"--work-dir", workDir,
		"--from", from,
		"--to", to,
		"--issue", issueID,
	)
	cmd.Env = withBinOnPath(sanitizedEnv(os.Environ()), shimDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("replay mutation subprocess: %s: %w\n%s", filepath.Base(mutationTranslatorBin), err, out)
	}
	return nil
}

// prependToPath creates a temporary directory containing a symlink named
// "bd" pointing at bin, so a subprocess that resolves "bd" via its own PATH
// lookup can be pointed at a specific pinned binary instead of whatever
// "bd" the ambient environment would otherwise resolve to. The caller must
// invoke the returned cleanup func.
func prependToPath(bin string) (dir string, cleanup func(), err error) {
	shimDir, err := os.MkdirTemp("", "driver-core-bd-shim-*")
	if err != nil {
		return "", nil, fmt.Errorf("prepend to path: create shim dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(shimDir) }

	absBin, err := filepath.Abs(bin)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("prepend to path: resolve %s: %w", bin, err)
	}
	if err := os.Symlink(absBin, filepath.Join(shimDir, "bd")); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("prepend to path: symlink bd -> %s: %w", absBin, err)
	}
	return shimDir, cleanup, nil
}

// withBinOnPath returns a copy of env with PATH rewritten so shimDir comes
// first. Any pre-existing PATH entries are filtered out rather than merely
// shadowed by a later one, since Go's exec.Cmd applies the LAST matching
// "KEY=" entry when duplicates exist -- filtering first makes that
// reliance explicit instead of incidental.
func withBinOnPath(env []string, shimDir string) []string {
	const prefix = "PATH="
	orig := ""
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			orig = strings.TrimPrefix(e, prefix)
			continue
		}
		out = append(out, e)
	}
	out = append(out, prefix+shimDir+string(os.PathListSeparator)+orig)
	return out
}

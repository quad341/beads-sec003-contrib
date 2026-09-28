// Command canary implements the Phase 4 nightly canary (be-hs42e.5.6,
// UC4/AF2): on its own schedule, it builds corpus-acquire (be-hs42e.5.1) and
// driver-core (be-hs42e.5.2) fresh from this checkout, runs them against a
// freshly acquired corpus clone and the integration branch's current tip,
// and turns the result into exactly one mayor summary plus one filed bead
// per new mismatch (UC3).
//
// canary never Go-imports corpus-acquire or driver-core -- both are
// separate `package main` binaries, built and invoked as subprocesses, the
// same decoupled-binary shape driver-core itself uses for oracle-query and
// mutation-translator.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type canaryConfig struct {
	HarnessRepo     string
	IntegrationRepo string
	CorpusDataDir   string
	Corpus          string
	WorkDir         string
	SchedulePath    string
	ScheduleID      string
	SampleSize      int
	RootBead        string
	SlingTarget     string
}

func main() {
	harnessRepo := flag.String("harness-repo", ".", "checkout containing scripts/corpus-acquire and scripts/driver-core to build from")
	integrationRepo := flag.String("integration-repo", "", "checkout of the integration branch under test (required)")
	corpusDataDir := flag.String("corpus-data-dir", "", "shared Dolt server data_dir containing the production corpus (required)")
	corpus := flag.String("corpus", "gascity", "corpus database name to replay against (architecture §3: gascity is the designated scale corpus)")
	workDir := flag.String("work-dir", "", "scratch directory for this run's corpus clone and driver-core work/out dirs (required)")
	schedulePath := flag.String("schedule-path", ".beads-canary/canary-schedule.json", "path to this canary's persisted schedule state")
	scheduleID := flag.String("schedule-id", "nightly-canary", "identifier recorded on the persisted schedule")
	sampleSize := flag.Int("sample-size", 200, "commits driver-core samples per run (0 = exhaustive; a canary stays sampled to fit a nightly time budget)")
	rootBead := flag.String("root-bead", "be-hs42e.5", "parent bead new mismatch bugs are filed under (AF2 step 3)")
	slingTarget := flag.String("sling-target", "beads/builder", "gc sling target for newly filed mismatch bugs (UC3)")
	flag.Parse()

	if *integrationRepo == "" || *corpusDataDir == "" || *workDir == "" {
		fmt.Fprintln(os.Stderr, "usage: canary --integration-repo DIR --corpus-data-dir DIR --work-dir DIR [--harness-repo DIR] [--corpus NAME] [--schedule-path PATH] [--sample-size N]")
		os.Exit(2)
	}

	cfg := canaryConfig{
		HarnessRepo: *harnessRepo, IntegrationRepo: *integrationRepo, CorpusDataDir: *corpusDataDir,
		Corpus: *corpus, WorkDir: *workDir, SchedulePath: *schedulePath, ScheduleID: *scheduleID,
		SampleSize: *sampleSize, RootBead: *rootBead, SlingTarget: *slingTarget,
	}

	if err := runCanary(context.Background(), cfg); err != nil {
		log.Fatalf("canary: %v", err)
	}
}

func runCanary(ctx context.Context, cfg canaryConfig) error {
	now := time.Now().UTC()
	runID := fmt.Sprintf("canary-%s", now.Format("20060102T150405Z"))

	integrationSHA, err := gitHEAD(ctx, cfg.IntegrationRepo)
	if err != nil {
		mailMayor(ctx, RunOutcome{RunID: runID, IntegrationSHA: "unknown", Status: "failed"})
		return fmt.Errorf("resolving integration SHA: %w", err)
	}

	sched := loadOrInitSchedule(cfg.SchedulePath, cfg.ScheduleID, cfg.Corpus, now)
	if !sched.Due(now) {
		fmt.Printf("canary: not due until %s, skipping\n", sched.NextDueAt.Format(time.RFC3339))
		return nil
	}

	raw, rawMismatches, err := replay(ctx, cfg, runID, integrationSHA)
	if err != nil {
		mailMayor(ctx, RunOutcome{RunID: runID, IntegrationSHA: integrationSHA, Status: "failed"})
		return fmt.Errorf("replay: %w", err)
	}

	filedPath := filepath.Join(filepath.Dir(cfg.SchedulePath), "filed-mismatches.json")
	filed := loadFiledMismatches(filedPath)

	outcome := raw
	outcome.NewMismatches = FilterNew(rawMismatches, filed)
	mailMayor(ctx, outcome)

	for _, m := range outcome.NewMismatches {
		beadID := fileMismatchBead(ctx, cfg.RootBead, m)
		if beadID == "" {
			continue
		}
		slingBead(ctx, cfg.SlingTarget, beadID)
		if filed == nil {
			filed = map[string]string{}
		}
		filed[Fingerprint(m)] = beadID
	}
	saveFiledMismatches(filedPath, filed)

	next, err := NextDueAfter(sched.Cadence, now)
	if err != nil {
		return fmt.Errorf("computing next due time: %w", err)
	}
	if err := SaveSchedule(cfg.SchedulePath, sched.Advance(runID, next)); err != nil {
		return fmt.Errorf("saving canary schedule: %w", err)
	}
	return nil
}

// replay builds corpus-acquire and driver-core fresh from cfg.HarnessRepo
// and runs them in sequence, returning the run's status/identity plus the
// raw (pre-UC3-filter) mismatches separately.
func replay(ctx context.Context, cfg canaryConfig, runID, integrationSHA string) (RunOutcome, []MismatchRecord, error) {
	toolsDir, err := os.MkdirTemp("", "canary-tools-*")
	if err != nil {
		return RunOutcome{}, nil, fmt.Errorf("creating tools dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(toolsDir) }()

	corpusAcquireBin := filepath.Join(toolsDir, "corpus-acquire")
	driverCoreBin := filepath.Join(toolsDir, "driver-core")
	if err := goBuild(ctx, cfg.HarnessRepo, "./scripts/corpus-acquire", corpusAcquireBin); err != nil {
		return RunOutcome{}, nil, fmt.Errorf("building corpus-acquire: %w", err)
	}
	if err := goBuild(ctx, cfg.HarnessRepo, "./scripts/driver-core", driverCoreBin); err != nil {
		return RunOutcome{}, nil, fmt.Errorf("building driver-core: %w", err)
	}

	runDir := filepath.Join(cfg.WorkDir, runID)
	corpusDir := filepath.Join(runDir, "corpus")
	replayWorkDir := filepath.Join(runDir, "replay-work")
	replayOutDir := filepath.Join(runDir, "replay-out")

	acquireCmd := exec.CommandContext(ctx, corpusAcquireBin,
		"-data-dir", cfg.CorpusDataDir, "-db", cfg.Corpus, "-dest", corpusDir)
	acquireCmd.Env = sanitizedEnv(os.Environ())
	if out, err := acquireCmd.CombinedOutput(); err != nil {
		return RunOutcome{}, nil, fmt.Errorf("corpus-acquire: %w\n%s", err, out)
	}

	driveCmd := exec.CommandContext(ctx, driverCoreBin,
		"-integration-ref", "HEAD",
		"-integration-repo", cfg.IntegrationRepo,
		"-oracle-data-dir", corpusDir,
		"-work-dir", replayWorkDir,
		"-out-dir", replayOutDir,
		"-sample-size", strconv.Itoa(cfg.SampleSize))
	driveCmd.Env = sanitizedEnv(os.Environ())
	if out, err := driveCmd.CombinedOutput(); err != nil {
		return RunOutcome{}, nil, fmt.Errorf("driver-core: %w\n%s", err, out)
	}

	status, err := lastReplayRunStatus(replayOutDir)
	if err != nil {
		return RunOutcome{}, nil, fmt.Errorf("reading replay run outcome: %w", err)
	}

	mismatches, err := ReadMismatches(filepath.Join(replayOutDir, "mismatches.jsonl"))
	if err != nil {
		return RunOutcome{}, nil, fmt.Errorf("reading mismatches: %w", err)
	}

	return RunOutcome{
		RunID: runID, IntegrationSHA: integrationSHA, Status: status, TotalMismatches: len(mismatches),
	}, mismatches, nil
}

// deniedEnvVars mirrors scripts/driver-core/env.go's list verbatim: ambient
// vars that must never reach a subprocess operating on dolt/bd state at a
// shared or production path (NFR3). canary spawns corpus-acquire and
// driver-core -- the same class of subprocess driver-core itself sanitizes
// before spawning oracle-query/mutation-translator -- so the same list
// applies here. mailMayor/fileMismatchBead/slingBead deliberately do NOT use
// this: those calls must reach the real project's bd/gc, not a stripped or
// ambient-redirected one.
var deniedEnvVars = map[string]bool{
	"BEADS_DOLT_SERVER_PORT":      true,
	"BEADS_DOLT_PORT":             true,
	"BEADS_ACTOR":                 true,
	"BD_ACTOR":                    true,
	"GT_ROOT":                     true,
	"BEADS_DIR":                   true,
	"BEADS_HOLDER_TOKEN":          true,
	"GC_BEADS_SCOPE_ROOT":         true,
	"BEADS_DOLT_AUTO_START":       true,
	"BEADS_DOLT_SYNC_CLI_REMOTES": true,
	"BEADS_BACKUP_ENABLED":        true,
}

// sanitizedEnv returns base with deniedEnvVars removed.
func sanitizedEnv(base []string) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if deniedEnvVars[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// lastReplayRunStatus returns the status field of the last record driver-core
// appended to replay_runs.jsonl -- the one this invocation just wrote.
func lastReplayRunStatus(outDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(outDir, "replay_runs.jsonl")) // #nosec G304 -- outDir is this process's own driver-core -out-dir
	if err != nil {
		return "", fmt.Errorf("reading replay_runs.jsonl: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	last := lines[len(lines)-1]
	if last == "" {
		return "", fmt.Errorf("replay_runs.jsonl has no entries")
	}
	var rr struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(last), &rr); err != nil {
		return "", fmt.Errorf("parsing last replay_runs.jsonl entry: %w", err)
	}
	return rr.Status, nil
}

// loadOrInitSchedule loads the persisted schedule, or bootstraps a fresh one
// due immediately if none loads cleanly -- a missing or corrupt state file
// must never block the canary from running, only cost it the "already ran
// recently" skip.
func loadOrInitSchedule(path, id, cloneID string, now time.Time) CanarySchedule {
	if s, err := LoadSchedule(path); err == nil {
		return s
	}
	return CanarySchedule{ScheduleID: id, CloneID: cloneID, Cadence: "nightly", NextDueAt: now}
}

func loadFiledMismatches(path string) map[string]string {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always this process's own persisted state file
	if err != nil {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

func saveFiledMismatches(path string, m map[string]string) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Printf("canary: marshaling filed-mismatches map: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("canary: creating filed-mismatches dir: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Printf("canary: saving filed-mismatches map: %v", err)
	}
}

func mailMayor(ctx context.Context, outcome RunOutcome) {
	subject, body := BuildMayorSummary(outcome)
	cmd := exec.CommandContext(ctx, "gc", "mail", "send", "mayor", subject, body, "--notify")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("canary: mailing mayor summary: %v\n%s", err, out)
	}
}

func fileMismatchBead(ctx context.Context, rootBead string, m MismatchRecord) string {
	cmd := exec.CommandContext(ctx, "bd", BuildBeadCreateArgs(rootBead, m)...)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("canary: filing mismatch bead: %v", err)
		return ""
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		log.Printf("canary: parsing filed bead id: %v", err)
		return ""
	}
	return created.ID
}

func slingBead(ctx context.Context, target, beadID string) {
	cmd := exec.CommandContext(ctx, "gc", BuildSlingArgs(target, beadID)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("canary: slinging %s: %v\n%s", beadID, err, out)
	}
}

// goBuild builds pkgPath (relative to dir) into outBin using whatever is
// currently checked out in dir. Mirrors driver-core/main.go's goBuild.
func goBuild(ctx context.Context, dir, pkgPath, outBin string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-o", outBin, pkgPath)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s in %s: %w\n%s", pkgPath, dir, err, out)
	}
	return nil
}

func gitHEAD(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

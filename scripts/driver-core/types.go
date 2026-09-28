// Command driver-core implements the Phase 4 replay-with-oracle driver
// (be-sodi8): AF1 single-commit orchestration and the UC2 full replay loop,
// wiring together oracle-query (be-3j80a) and mutation-translator (be-2cp1d)
// as subprocesses, comparing their output, and persisting results.
package main

import (
	"encoding/json"
	"time"
)

// ReplayRun is the ERD's REPLAY_RUN entity (be-hs42e.5 §5): one invocation of
// the harness, pinning the exact integration build under test (NFR4).
type ReplayRun struct {
	ID             string    `json:"id"`
	IntegrationRef string    `json:"integration_ref"`
	IntegrationSHA string    `json:"integration_sha"`
	Mode           string    `json:"mode"` // "exhaustive" | "sampled"
	SampleSize     int       `json:"sample_size,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	Status         string    `json:"status"` // "running" | "completed" | "failed"
}

// CommitReplayResult is the ERD's COMMIT_REPLAY_RESULT entity: the outcome of
// replaying one historical commit's mutation for one issue and comparing it
// against the oracle. Written for every (commit, issue) pair the run visits,
// matched or not (be-sodi8 AC2).
type CommitReplayResult struct {
	RunID         string `json:"run_id"`
	SourceCommit  string `json:"source_commit"`
	IssueID       string `json:"issue_id"`
	MutationKind  string `json:"mutation_kind"`
	Matched       bool   `json:"matched"`
	OracleHash    string `json:"oracle_hash"`
	CandidateHash string `json:"candidate_hash"`
}

// Mismatch is the ERD's MISMATCH entity: full repro context for a
// non-matching comparison (be-sodi8 AC2).
type Mismatch struct {
	RunID        string          `json:"run_id"`
	SourceCommit string          `json:"source_commit"`
	IssueID      string          `json:"issue_id"`
	Category     string          `json:"category"`
	ExpectedJSON json.RawMessage `json:"expected_json"`
	ActualJSON   json.RawMessage `json:"actual_json"`
}

// MetricSample is the ERD's METRIC_SAMPLE entity: one raw measurement or
// aggregate. Driver-core emits raw samples directly (be-sodi8 AC4:
// "storage_bytes", "write_latency_ms"); be-hs42e.5.3 adds growth samples
// ("issue_versions_bytes", "retained_payload_bytes", "tombstone_count",
// tagged Phase "baseline"/"final") and percentile aggregates
// ("write_latency_p50_ms"/"p95_ms"/"p99_ms", tagged Phase "aggregate").
type MetricSample struct {
	RunID     string    `json:"run_id"`
	Name      string    `json:"name"`
	Phase     string    `json:"phase,omitempty"`
	Value     float64   `json:"value"`
	SampledAt time.Time `json:"sampled_at"`
}

package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

// Percentile computes p (0-100) over values using the nearest-rank method:
// rank = ceil(p/100 * N) clamped to [1, N], value = sorted[rank-1].
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	rank := int(math.Ceil(p / 100 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}

// IssueVersionsBytes sums durable_state length over every revision of every
// issue -- full historical accumulation, live or tombstoned.
func IssueVersionsBytes(ctx context.Context, dataDir string) (int64, error) {
	return scalarInt64Query(ctx, dataDir, "SELECT COALESCE(SUM(LENGTH(durable_state)), 0) FROM issue_versions")
}

// RetainedPayloadBytes sums durable_state length over only the latest
// revision of each still-live issue -- the payload a compaction pass could
// not discard without losing current state.
func RetainedPayloadBytes(ctx context.Context, dataDir string) (int64, error) {
	query := `
SELECT COALESCE(SUM(LENGTH(iv.durable_state)), 0)
FROM issue_versions iv
JOIN (
    SELECT issue_id, MAX(revision) AS max_revision
    FROM issue_versions
    GROUP BY issue_id
) latest ON latest.issue_id = iv.issue_id AND latest.max_revision = iv.revision
JOIN issues i ON i.id = iv.issue_id`
	return scalarInt64Query(ctx, dataDir, query)
}

// TombstoneCount counts distinct issue_ids present in issue_versions with no
// matching live row in issues. issue_versions carries no FK back to issues,
// so a deleted issue's history survives as an orphan rather than cascading.
func TombstoneCount(ctx context.Context, dataDir string) (int64, error) {
	query := `
SELECT COUNT(DISTINCT iv.issue_id)
FROM issue_versions iv
LEFT JOIN issues i ON i.id = iv.issue_id
WHERE i.id IS NULL`
	return scalarInt64Query(ctx, dataDir, query)
}

func scalarInt64Query(ctx context.Context, dataDir, query string) (int64, error) {
	header, rows, err := doltQuery(ctx, dataDir, query)
	if err != nil {
		return 0, fmt.Errorf("scalar query: %w", err)
	}
	if len(rows) != 1 || len(header) != 1 {
		return 0, fmt.Errorf("scalar query: expected 1x1 result, got %d rows x %d cols", len(rows), len(header))
	}
	v, err := strconv.ParseInt(rows[0][0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("scalar query: parsing %q: %w", rows[0][0], err)
	}
	return v, nil
}

// CaptureGrowthMetrics bundles the three storage-growth queries into
// MetricSample rows tagged with runID and phase (e.g. "baseline"/"final") so
// callers can compute growth as a delta between two captures of the same run.
func CaptureGrowthMetrics(ctx context.Context, dataDir, runID, phase string, now time.Time) ([]MetricSample, error) {
	versionsBytes, err := IssueVersionsBytes(ctx, dataDir)
	if err != nil {
		return nil, fmt.Errorf("capture growth metrics: %w", err)
	}
	retainedBytes, err := RetainedPayloadBytes(ctx, dataDir)
	if err != nil {
		return nil, fmt.Errorf("capture growth metrics: %w", err)
	}
	tombstones, err := TombstoneCount(ctx, dataDir)
	if err != nil {
		return nil, fmt.Errorf("capture growth metrics: %w", err)
	}
	return []MetricSample{
		{RunID: runID, Name: "issue_versions_bytes", Phase: phase, Value: float64(versionsBytes), SampledAt: now},
		{RunID: runID, Name: "retained_payload_bytes", Phase: phase, Value: float64(retainedBytes), SampledAt: now},
		{RunID: runID, Name: "tombstone_count", Phase: phase, Value: float64(tombstones), SampledAt: now},
	}, nil
}

// AggregateWriteLatency computes p50/p95/p99 over the write_latency_ms
// samples already collected for runID, scoped to that run so a multi-run
// store never blends unrelated runs' latencies. Returns nil when runID has
// no write_latency_ms samples rather than fabricating percentile rows.
func AggregateWriteLatency(runID string, samples []MetricSample, now time.Time) []MetricSample {
	var latencies []float64
	for _, s := range samples {
		if s.RunID == runID && s.Name == "write_latency_ms" {
			latencies = append(latencies, s.Value)
		}
	}
	if len(latencies) == 0 {
		return nil
	}
	return []MetricSample{
		{RunID: runID, Name: "write_latency_p50_ms", Phase: "aggregate", Value: Percentile(latencies, 50), SampledAt: now},
		{RunID: runID, Name: "write_latency_p95_ms", Phase: "aggregate", Value: Percentile(latencies, 95), SampledAt: now},
		{RunID: runID, Name: "write_latency_p99_ms", Phase: "aggregate", Value: Percentile(latencies, 99), SampledAt: now},
	}
}

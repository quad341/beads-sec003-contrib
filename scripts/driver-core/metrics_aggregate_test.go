package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestPercentile_NearestRank pins the nearest-rank method be-hs42e.5.3 settled
// on: rank = ceil(p/100 * N) clamped to [1, N], value = sorted[rank-1]. 1..100
// is the cleanest fixture for this method: percentile p lands exactly on
// value p for every p in this range.
func TestPercentile_NearestRank(t *testing.T) {
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i + 1)
	}
	for _, tc := range []struct {
		p    float64
		want float64
	}{
		{50, 50},
		{95, 95},
		{99, 99},
	} {
		if got := Percentile(values, tc.p); got != tc.want {
			t.Errorf("Percentile(1..100, %v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

// TestPercentile_UnsortedInputIsSorted proves Percentile sorts its own copy
// rather than assuming a pre-sorted caller.
func TestPercentile_UnsortedInputIsSorted(t *testing.T) {
	values := []float64{30, 10, 50, 20, 40}
	if got := Percentile(values, 50); got != 30 {
		t.Errorf("Percentile(unsorted, 50) = %v, want 30", got)
	}
}

// TestPercentile_EmptyReturnsZero documents the empty-input edge case: a run
// with zero write-latency samples must not panic or divide by zero.
func TestPercentile_EmptyReturnsZero(t *testing.T) {
	if got := Percentile(nil, 50); got != 0 {
		t.Errorf("Percentile(nil, 50) = %v, want 0", got)
	}
}

// TestPercentile_SingleValueClampsRank exercises the rank-clamping edge: N=1
// must return the single value for every p, not index out of range.
func TestPercentile_SingleValueClampsRank(t *testing.T) {
	for _, p := range []float64{1, 50, 99, 100} {
		if got := Percentile([]float64{42}, p); got != 42 {
			t.Errorf("Percentile([42], %v) = %v, want 42", p, got)
		}
	}
}

// insertVersion inserts one synthetic issue_versions row directly via dolt
// sql, bypassing bd entirely: issueops.RecordVersionInTx's dual-write is off
// by default in every build so far (version_history.go's own doc comment), so
// a plain bd create/update against this fixture would leave issue_versions
// empty. issue_id carries no foreign key to issues (confirmed against every
// migration under internal/storage/schema/migrations), so issueID need not
// exist in the issues table -- deliberately, for the tombstone tests, where
// it must not.
func insertVersion(t *testing.T, dataDir, issueID string, revision int, payload string) {
	t.Helper()
	query := fmt.Sprintf(
		`INSERT INTO issue_versions (issue_id, revision, epoch, durable_state, change_actor, change_agent, change_message, change_at, attribution_status) VALUES (%s, %d, 1, %s, '', '', '', NOW(), 'unknown')`,
		sqlQuote(issueID), revision, sqlQuote(payload))
	runDolt(t, dataDir, "sql", "-q", query)
}

// TestIssueVersionsBytes_SumsAllRevisionsAcrossAllIssues is FR5/be-hs42e.5
// §10's full historical-accumulation dimension: every revision of every
// issue counts, live or not.
func TestIssueVersionsBytes_SumsAllRevisionsAcrossAllIssues(t *testing.T) {
	requireHeadBd(t)
	requireDolt(t)
	dir := initBdProjectWith(t, "proj", headBd)
	dd := dataDir(t, dir)

	p1, p2, p3 := `{"a":1}`, `{"a":22}`, `{"a":333}`
	insertVersion(t, dd, "issue-1", 1, p1)
	insertVersion(t, dd, "issue-1", 2, p2)
	insertVersion(t, dd, "issue-2", 1, p3)

	got, err := IssueVersionsBytes(context.Background(), dd)
	if err != nil {
		t.Fatalf("IssueVersionsBytes: %v", err)
	}
	want := int64(len(p1) + len(p2) + len(p3))
	if got != want {
		t.Errorf("IssueVersionsBytes = %d, want %d", got, want)
	}
}

// TestRetainedPayloadBytes_OnlyLatestRevisionOfLiveIssues distinguishes
// RetainedPayloadBytes from IssueVersionsBytes: only the highest revision per
// issue counts, and only for issues still live in the issues table -- the
// payload a compaction pass could not discard without losing current state.
func TestRetainedPayloadBytes_OnlyLatestRevisionOfLiveIssues(t *testing.T) {
	requireHeadBd(t)
	requireDolt(t)
	dir := initBdProjectWith(t, "proj", headBd)
	dd := dataDir(t, dir)

	outA := runBdBin(t, headBd, dir, "create", "Issue A", "--type", "task", "--json")
	idA := jsonID(t, outA)
	outB := runBdBin(t, headBd, dir, "create", "Issue B", "--type", "task", "--json")
	idB := jsonID(t, outB)

	pA1, pA2 := `{"a":1}`, `{"a":222222}`
	insertVersion(t, dd, idA, 1, pA1)
	insertVersion(t, dd, idA, 2, pA2)
	pB1 := `{"b":1}`
	insertVersion(t, dd, idB, 1, pB1)
	// Orphan: never created via bd create, so absent from issues -- must not
	// count towards a "live" issue's retained payload.
	pC1 := `{"c":1}`
	insertVersion(t, dd, "issue-orphan", 1, pC1)

	got, err := RetainedPayloadBytes(context.Background(), dd)
	if err != nil {
		t.Fatalf("RetainedPayloadBytes: %v", err)
	}
	want := int64(len(pA2) + len(pB1))
	if got != want {
		t.Errorf("RetainedPayloadBytes = %d, want %d (pA1=%d excluded as a stale revision, pC1=%d excluded as orphaned)",
			got, want, len(pA1), len(pC1))
	}
}

// TestTombstoneCount_CountsDistinctOrphanedIssueIDs grounds "tombstone" in the
// current schema rather than the not-yet-implemented #5898 erasure design:
// internal/storage/issueops.DeleteIssueInTx's deleteIssueRowInTx deletes only
// the issues row, and issue_versions carries no ON DELETE CASCADE FK back to
// issues (delete_cascade_tables_test.go enumerates every real cascade target
// and does not include it), so a deleted issue's history survives as an
// orphaned issue_id. Counts distinct issue_ids, not rows: an orphan with
// several surviving revisions is still one tombstoned issue.
func TestTombstoneCount_CountsDistinctOrphanedIssueIDs(t *testing.T) {
	requireHeadBd(t)
	requireDolt(t)
	dir := initBdProjectWith(t, "proj", headBd)
	dd := dataDir(t, dir)

	outA := runBdBin(t, headBd, dir, "create", "Issue A", "--type", "task", "--json")
	idA := jsonID(t, outA)
	insertVersion(t, dd, idA, 1, `{"a":1}`)

	insertVersion(t, dd, "issue-orphan-1", 1, `{"c":1}`)
	insertVersion(t, dd, "issue-orphan-2", 1, `{"d":1}`)
	insertVersion(t, dd, "issue-orphan-2", 2, `{"d":2}`)

	got, err := TombstoneCount(context.Background(), dd)
	if err != nil {
		t.Fatalf("TombstoneCount: %v", err)
	}
	if got != 2 {
		t.Errorf("TombstoneCount = %d, want 2 (issue-orphan-1 and issue-orphan-2, each counted once)", got)
	}
}

// TestCaptureGrowthMetrics_TagsRunIDAndPhase pins the bundling function
// run.go calls at baseline and again at run-end (UC2 step M): the three
// growth queries, tagged with the caller's run_id and phase and a single
// captured_at.
func TestCaptureGrowthMetrics_TagsRunIDAndPhase(t *testing.T) {
	requireHeadBd(t)
	requireDolt(t)
	dir := initBdProjectWith(t, "proj", headBd)
	dd := dataDir(t, dir)
	insertVersion(t, dd, "issue-1", 1, `{"a":1}`)

	now := time.Now().UTC()
	samples, err := CaptureGrowthMetrics(context.Background(), dd, "run-1", "final", now)
	if err != nil {
		t.Fatalf("CaptureGrowthMetrics: %v", err)
	}

	wantNames := map[string]bool{"issue_versions_bytes": false, "retained_payload_bytes": false, "tombstone_count": false}
	for _, s := range samples {
		if s.RunID != "run-1" {
			t.Errorf("sample %q: RunID = %q, want run-1", s.Name, s.RunID)
		}
		if s.Phase != "final" {
			t.Errorf("sample %q: Phase = %q, want final", s.Name, s.Phase)
		}
		if !s.SampledAt.Equal(now) {
			t.Errorf("sample %q: SampledAt = %v, want %v", s.Name, s.SampledAt, now)
		}
		if _, ok := wantNames[s.Name]; !ok {
			t.Errorf("unexpected sample name %q", s.Name)
		}
		wantNames[s.Name] = true
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("CaptureGrowthMetrics did not return a %q sample", name)
		}
	}
}

// TestAggregateWriteLatency_ComputesPercentilesPerRun is UC2 step M's other
// half: percentile aggregation over raw write_latency_ms samples already
// collected during replay, scoped to one run_id so a multi-run store never
// blends unrelated runs' latencies.
func TestAggregateWriteLatency_ComputesPercentilesPerRun(t *testing.T) {
	var samples []MetricSample
	for i := 1; i <= 100; i++ {
		samples = append(samples, MetricSample{RunID: "run-1", Name: "write_latency_ms", Value: float64(i)})
	}
	// A second run's samples must not leak into run-1's percentiles.
	samples = append(samples, MetricSample{RunID: "run-2", Name: "write_latency_ms", Value: 9999})
	// A differently-named sample for the same run must not leak in either.
	samples = append(samples, MetricSample{RunID: "run-1", Name: "storage_bytes", Value: 123})

	now := time.Now().UTC()
	got := AggregateWriteLatency("run-1", samples, now)

	want := map[string]float64{"write_latency_p50_ms": 50, "write_latency_p95_ms": 95, "write_latency_p99_ms": 99}
	if len(got) != len(want) {
		t.Fatalf("AggregateWriteLatency returned %d samples, want %d: %+v", len(got), len(want), got)
	}
	for _, s := range got {
		if s.RunID != "run-1" {
			t.Errorf("sample %q: RunID = %q, want run-1", s.Name, s.RunID)
		}
		if s.Phase != "aggregate" {
			t.Errorf("sample %q: Phase = %q, want aggregate", s.Name, s.Phase)
		}
		if !s.SampledAt.Equal(now) {
			t.Errorf("sample %q: SampledAt = %v, want %v", s.Name, s.SampledAt, now)
		}
		wantVal, ok := want[s.Name]
		if !ok {
			t.Errorf("unexpected sample name %q", s.Name)
			continue
		}
		if s.Value != wantVal {
			t.Errorf("sample %q: Value = %v, want %v", s.Name, s.Value, wantVal)
		}
	}
}

// TestAggregateWriteLatency_ReturnsNilWhenNoSamples: a run with no
// write_latency_ms samples (e.g. zero mutating commits replayed) must not
// fabricate percentile rows.
func TestAggregateWriteLatency_ReturnsNilWhenNoSamples(t *testing.T) {
	samples := []MetricSample{{RunID: "run-1", Name: "storage_bytes", Value: 1}}
	if got := AggregateWriteLatency("run-1", samples, time.Now().UTC()); got != nil {
		t.Errorf("AggregateWriteLatency = %+v, want nil", got)
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStore_WriteReplayRun_AppendsJSONLine(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	run := ReplayRun{ID: "replay-run-abc", IntegrationRef: "HEAD", IntegrationSHA: "deadbeef", Mode: "exhaustive", StartedAt: time.Now().UTC(), Status: "completed"}
	if err := store.WriteReplayRun(run); err != nil {
		t.Fatalf("WriteReplayRun: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "replay_runs.jsonl"))
	if err != nil {
		t.Fatalf("reading replay_runs.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(lines), data)
	}
	var got ReplayRun
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshaling written line: %v", err)
	}
	if got.ID != run.ID || got.IntegrationSHA != run.IntegrationSHA {
		t.Errorf("round-tripped run = %+v, want %+v", got, run)
	}
}

func TestStore_WriteReplayRun_AppendsAcrossMultipleCalls(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.WriteReplayRun(ReplayRun{ID: "run-1"}); err != nil {
		t.Fatalf("WriteReplayRun 1: %v", err)
	}
	if err := store.WriteReplayRun(ReplayRun{ID: "run-2"}); err != nil {
		t.Fatalf("WriteReplayRun 2: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "replay_runs.jsonl"))
	if err != nil {
		t.Fatalf("reading replay_runs.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), data)
	}
}

func TestStore_WriteCommitReplayResult(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	crr := CommitReplayResult{RunID: "run-1", SourceCommit: "abc123", IssueID: "X-1", MutationKind: "create", Matched: true, OracleHash: "h1", CandidateHash: "h1"}
	if err := store.WriteCommitReplayResult(crr); err != nil {
		t.Fatalf("WriteCommitReplayResult: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "commit_replay_results.jsonl"))
	if err != nil {
		t.Fatalf("reading commit_replay_results.jsonl: %v", err)
	}
	var got CommitReplayResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("unmarshaling written line: %v", err)
	}
	if got.IssueID != "X-1" || got.MutationKind != "create" {
		t.Errorf("round-tripped result = %+v", got)
	}
}

func TestStore_WriteMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mm := Mismatch{RunID: "run-1", SourceCommit: "abc123", IssueID: "X-1", Category: "attribution", ExpectedJSON: json.RawMessage(`{"a":1}`), ActualJSON: json.RawMessage(`{"a":2}`)}
	if err := store.WriteMismatch(mm); err != nil {
		t.Fatalf("WriteMismatch: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "mismatches.jsonl"))
	if err != nil {
		t.Fatalf("reading mismatches.jsonl: %v", err)
	}
	var got Mismatch
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("unmarshaling written line: %v", err)
	}
	if got.Category != "attribution" || string(got.ExpectedJSON) != `{"a":1}` {
		t.Errorf("round-tripped mismatch = %+v", got)
	}
}

func TestStore_WriteMetricSample(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ms := MetricSample{RunID: "run-1", Name: "storage_bytes", Value: 12345, SampledAt: time.Now().UTC()}
	if err := store.WriteMetricSample(ms); err != nil {
		t.Fatalf("WriteMetricSample: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "metric_samples.jsonl"))
	if err != nil {
		t.Fatalf("reading metric_samples.jsonl: %v", err)
	}
	var got MetricSample
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("unmarshaling written line: %v", err)
	}
	if got.Name != "storage_bytes" || got.Value != 12345 {
		t.Errorf("round-tripped metric sample = %+v", got)
	}
}

func TestNewStore_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: %s already exists", dir)
	}
	if _, err := NewStore(dir); err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("NewStore did not create directory %s: err=%v", dir, err)
	}
}

func TestDirSize_SumsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("123"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}
	got, err := dirSize(dir)
	if err != nil {
		t.Fatalf("dirSize: %v", err)
	}
	if got != 8 {
		t.Errorf("dirSize = %d, want 8", got)
	}
}

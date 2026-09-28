package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store is a dependency-free, append-only JSON-Lines persistence layer for
// the ERD's four entities. JSONL rather than a SQL engine because NFR1/NFR2
// forbid ever opening a write-capable connection to the shared Dolt server,
// and a SQL-engine choice here could be mistaken for a step toward one.
type Store struct {
	dir string
}

// NewStore creates dir (including any missing parents) if needed and
// returns a Store that writes into it.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating store directory %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) appendJSONLine(filename string, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling %s entry: %w", filename, err)
	}
	f, err := os.OpenFile(filepath.Join(s.dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- filename is always one of this file's own *.jsonl literals, never external input
	if err != nil {
		return fmt.Errorf("opening %s: %w", filename, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("writing %s: %w", filename, err)
	}
	return nil
}

func (s *Store) WriteReplayRun(run ReplayRun) error {
	return s.appendJSONLine("replay_runs.jsonl", run)
}

func (s *Store) WriteCommitReplayResult(crr CommitReplayResult) error {
	return s.appendJSONLine("commit_replay_results.jsonl", crr)
}

func (s *Store) WriteMismatch(mm Mismatch) error {
	return s.appendJSONLine("mismatches.jsonl", mm)
}

func (s *Store) WriteMetricSample(ms MetricSample) error {
	return s.appendJSONLine("metric_samples.jsonl", ms)
}

// dirSize returns the sum of regular file sizes under dir, recursively.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("computing directory size for %s: %w", dir, err)
	}
	return total, nil
}

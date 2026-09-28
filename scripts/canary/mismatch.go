package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MismatchRecord mirrors driver-core's Mismatch JSONL shape (be-sodi8,
// scripts/driver-core/types.go) so this package can read driver-core's
// mismatches.jsonl output without Go-importing it (decoupled-binary
// design, be-hs42e.5.6 design decision 3).
type MismatchRecord struct {
	RunID        string          `json:"run_id"`
	SourceCommit string          `json:"source_commit"`
	IssueID      string          `json:"issue_id"`
	Category     string          `json:"category"`
	ExpectedJSON json.RawMessage `json:"expected_json,omitempty"`
	ActualJSON   json.RawMessage `json:"actual_json,omitempty"`
}

// ReadMismatches parses a driver-core mismatches.jsonl file. A missing file
// means the run found zero mismatches (driver-core's store only creates the
// file on the first WriteMismatch call), not an error.
func ReadMismatches(path string) ([]MismatchRecord, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always this process's own driver-core out-dir file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading mismatches %s: %w", path, err)
	}

	var records []MismatchRecord
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m MismatchRecord
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			return nil, fmt.Errorf("parsing mismatch line in %s: %w", path, err)
		}
		records = append(records, m)
	}
	return records, nil
}

// Fingerprint identifies the underlying regression a mismatch represents,
// deliberately excluding RunID: the same regression recurring across
// nightly runs must fingerprint identically, or UC3's already-filed check
// would re-file it every night.
func Fingerprint(m MismatchRecord) string {
	return m.SourceCommit + "|" + m.IssueID + "|" + m.Category
}

// FilterNew returns the records not already present (by Fingerprint) in
// filed, the fingerprint -> filed-bead-id map UC3 persists. A nil map
// (nothing filed yet) means every record is new.
func FilterNew(records []MismatchRecord, filed map[string]string) []MismatchRecord {
	var out []MismatchRecord
	for _, r := range records {
		if _, ok := filed[Fingerprint(r)]; !ok {
			out = append(out, r)
		}
	}
	return out
}

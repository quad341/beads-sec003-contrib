// Comparator canonicalizes two JSON payloads per RFC 8785 (JCS), hashes
// them, and on mismatch classifies the difference for triage. It reuses
// Phase 0's canonicalization dependency (github.com/gowebpki/jcs, the same
// library internal/storage/issueops/version_history.go and dependencies.go
// depend on) rather than reimporting that package: package main can't be
// imported cross-package, and importing it would pull in Dolt/DB coupling
// this comparator must stay free of.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gowebpki/jcs"
)

// Result is COMMIT_REPLAY_RESULT: the outcome of comparing an oracle
// payload against a candidate payload.
type Result struct {
	Matched       bool      `json:"matched"`
	OracleHash    string    `json:"oracle_hash"`
	CandidateHash string    `json:"candidate_hash"`
	Mismatch      *Mismatch `json:"mismatch,omitempty"`
}

// Mismatch is MISMATCH: enough context to triage a non-matching comparison
// without re-deriving it -- category plus both payloads verbatim (raw, not
// canonicalized, so a human or driver sees exactly what was sent).
type Mismatch struct {
	Category     string          `json:"category"`
	ExpectedJSON json.RawMessage `json:"expected_json"`
	ActualJSON   json.RawMessage `json:"actual_json"`
}

// Canonicalize transforms a JSON payload per RFC 8785 (JCS): object keys
// sorted recursively, numbers in ECMA-262 form. Same library and
// error-wrapping style as canonicalDurableState (version_history.go) and
// DependencyMetadataEqual (dependencies.go) -- this is that same dependency
// applied directly to already-serialized JSON, not a reimplementation.
func Canonicalize(payload []byte) ([]byte, error) {
	canonical, err := jcs.Transform(payload)
	if err != nil {
		return nil, fmt.Errorf("canonicalize (RFC 8785): %w", err)
	}
	return canonical, nil
}

// Hash returns the lowercase-hex SHA-256 digest of canonical.
func Hash(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// Compare canonicalizes and hashes both payloads; Matched reports whether
// the two hashes agree. On mismatch, Mismatch carries both payloads
// verbatim plus a best-effort category from classifyMismatch.
func Compare(oracleJSON, candidateJSON []byte) (Result, error) {
	oracleCanon, err := Canonicalize(oracleJSON)
	if err != nil {
		return Result{}, fmt.Errorf("canonicalize oracle payload: %w", err)
	}
	candidateCanon, err := Canonicalize(candidateJSON)
	if err != nil {
		return Result{}, fmt.Errorf("canonicalize candidate payload: %w", err)
	}

	oracleHash := Hash(oracleCanon)
	candidateHash := Hash(candidateCanon)

	result := Result{
		Matched:       oracleHash == candidateHash,
		OracleHash:    oracleHash,
		CandidateHash: candidateHash,
	}
	if !result.Matched {
		result.Mismatch = &Mismatch{
			Category:     classifyMismatch(oracleJSON, candidateJSON),
			ExpectedJSON: json.RawMessage(oracleJSON),
			ActualJSON:   json.RawMessage(candidateJSON),
		}
	}
	return result, nil
}

// Field-name synonym sets grounded in the real schema (read directly, not
// guessed): internal/storage/schema/migrations/0001_create_issues.up.sql,
// 0002_create_dependencies.up.sql, and version_history.go's issue_versions
// row shape (epoch, revision, change_actor/change_agent,
// attribution_status). A mismatch payload may be shaped like either a raw
// issues row or an issue_versions row, so each set hedges across both.
var (
	depEdgeKeys = map[string]bool{
		"dependencies": true,
		"depends_on":   true,
		"dep_ids":      true,
		"blocked_by":   true,
		"deps":         true,
	}
	versionCountKeys = map[string]bool{
		"content_hash":  true,
		"version":       true,
		"version_count": true,
		"revision":      true,
	}
	attributionKeys = map[string]bool{
		"created_by":         true,
		"owner":              true,
		"actor":              true,
		"sender":             true,
		"assignee":           true,
		"closed_by_session":  true,
		"change_actor":       true,
		"change_agent":       true,
		"attribution_status": true,
	}
	epochKeys = map[string]bool{
		"epoch": true,
	}
	timestampKeys = map[string]bool{
		"created_at":    true,
		"updated_at":    true,
		"closed_at":     true,
		"last_activity": true,
		"due_at":        true,
		"defer_until":   true,
		"compacted_at":  true,
	}
)

// classifyMismatch returns a best-effort category for a mismatch between
// two JSON object payloads, by comparing top-level keys whose values differ
// (present on only one side counts as differing). Rule order matters:
// dep-edge and version-count are checked before the as-of-mismatch
// catch-all, so a payload that differs in both a dependency edge and a
// timestamp is still reported as dep-edge, the more actionable signal.
// Falls back to "unclassified" -- deliberately not "no-op-false-positive",
// which would assert caller intent this function has no way to know -- when
// no rule matches; the bead's category list is "e.g.", not a closed enum.
func classifyMismatch(oracleJSON, candidateJSON []byte) string {
	var oracle, candidate map[string]any
	// Errors are unreachable here: Compare already proved both payloads
	// parse as JSON (via Canonicalize) before calling this function.
	_ = json.Unmarshal(oracleJSON, &oracle)
	_ = json.Unmarshal(candidateJSON, &candidate)

	differing := map[string]bool{}
	for k, v := range oracle {
		if cv, ok := candidate[k]; !ok || !reflect.DeepEqual(v, cv) {
			differing[k] = true
		}
	}
	for k := range candidate {
		if _, ok := oracle[k]; !ok {
			differing[k] = true
		}
	}

	switch {
	case anyKeyIn(differing, depEdgeKeys):
		return "dep-edge"
	case anyKeyIn(differing, versionCountKeys):
		return "version-count"
	case anyKeyIn(differing, attributionKeys):
		return "attribution"
	case anyKeyIn(differing, epochKeys):
		return "epoch"
	case len(differing) > 0 && allKeysIn(differing, timestampKeys):
		return "as-of-mismatch"
	default:
		return "unclassified"
	}
}

func anyKeyIn(keys, set map[string]bool) bool {
	for k := range keys {
		if set[k] {
			return true
		}
	}
	return false
}

func allKeysIn(keys, set map[string]bool) bool {
	for k := range keys {
		if !set[k] {
			return false
		}
	}
	return true
}

func main() {}

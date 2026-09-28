package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gowebpki/jcs"
)

// CompareResult is the outcome of comparing an oracle payload against a
// candidate payload for one (commit, issue) pair.
type CompareResult struct {
	Matched       bool
	OracleHash    string
	CandidateHash string
	Mismatch      *ComparisonMismatch
}

// ComparisonMismatch carries enough context to triage a non-matching
// comparison without re-deriving it: a best-effort category plus both
// payloads verbatim (raw, not canonicalized, so a human or the MISMATCH
// entity sees exactly what was compared).
type ComparisonMismatch struct {
	Category     string
	ExpectedJSON json.RawMessage
	ActualJSON   json.RawMessage
}

// Canonicalize transforms a JSON payload per RFC 8785 (JCS): object keys
// sorted recursively, numbers in ECMA-262 form. Ported from
// scripts/comparator (be-f2gog), which itself reuses the same
// github.com/gowebpki/jcs dependency internal/storage/issueops's
// version_history.go and dependencies.go depend on.
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
func Compare(oracleJSON, candidateJSON []byte) (CompareResult, error) {
	oracleCanon, err := Canonicalize(oracleJSON)
	if err != nil {
		return CompareResult{}, fmt.Errorf("canonicalize oracle payload: %w", err)
	}
	candidateCanon, err := Canonicalize(candidateJSON)
	if err != nil {
		return CompareResult{}, fmt.Errorf("canonicalize candidate payload: %w", err)
	}

	oracleHash := Hash(oracleCanon)
	candidateHash := Hash(candidateCanon)

	result := CompareResult{
		Matched:       oracleHash == candidateHash,
		OracleHash:    oracleHash,
		CandidateHash: candidateHash,
	}
	if !result.Matched {
		result.Mismatch = &ComparisonMismatch{
			Category:     classifyMismatch(oracleJSON, candidateJSON),
			ExpectedJSON: json.RawMessage(oracleJSON),
			ActualJSON:   json.RawMessage(candidateJSON),
		}
	}
	return result, nil
}

// Field-name synonym sets, ported verbatim from scripts/comparator
// (be-f2gog), grounded in the real schema: internal/storage/schema's issues
// and dependencies migrations, and version_history.go's issue_versions row
// shape (epoch, revision, change_actor/change_agent, attribution_status). A
// mismatch payload may be shaped like either a raw issues row or an
// issue_versions row, so each set hedges across both.
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

// classifyMismatch returns a best-effort category for a mismatch between two
// JSON object payloads, by comparing top-level keys whose values differ
// (present on only one side counts as differing). Rule order matters:
// dep-edge and version-count are checked before the as-of-mismatch
// catch-all, so a payload that differs in both a dependency edge and a
// timestamp is still reported as dep-edge, the more actionable signal. Falls
// back to "unclassified" when no rule matches.
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

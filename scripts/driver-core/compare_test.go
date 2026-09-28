package main

import "testing"

func TestCompare_MatchedOnIdenticalPayloads(t *testing.T) {
	payload := []byte(`{"id":"X-1","title":"hello","priority":2}`)
	result, err := Compare(payload, payload)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !result.Matched {
		t.Fatalf("expected Matched=true for identical payloads")
	}
	if result.OracleHash == "" || result.CandidateHash == "" {
		t.Fatalf("expected non-empty hashes, got oracle=%q candidate=%q", result.OracleHash, result.CandidateHash)
	}
	if result.OracleHash != result.CandidateHash {
		t.Fatalf("expected equal hashes for identical payloads, got %q vs %q", result.OracleHash, result.CandidateHash)
	}
	if result.Mismatch != nil {
		t.Fatalf("expected nil Mismatch on match, got %+v", result.Mismatch)
	}
}

func TestCompare_MatchedIgnoringKeyOrder(t *testing.T) {
	oracle := []byte(`{"a":1,"b":2}`)
	candidate := []byte(`{"b":2,"a":1}`)
	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !result.Matched {
		t.Fatalf("expected Matched=true regardless of key order (RFC 8785 canonicalization)")
	}
}

func TestCompare_MismatchOnDifferentValues(t *testing.T) {
	oracle := []byte(`{"id":"X-1","title":"hello"}`)
	candidate := []byte(`{"id":"X-1","title":"goodbye"}`)
	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if result.Matched {
		t.Fatalf("expected Matched=false for differing payloads")
	}
	if result.Mismatch == nil {
		t.Fatalf("expected non-nil Mismatch on mismatch")
	}
	if string(result.Mismatch.ExpectedJSON) != string(oracle) {
		t.Errorf("Mismatch.ExpectedJSON = %s, want %s", result.Mismatch.ExpectedJSON, oracle)
	}
	if string(result.Mismatch.ActualJSON) != string(candidate) {
		t.Errorf("Mismatch.ActualJSON = %s, want %s", result.Mismatch.ActualJSON, candidate)
	}
}

func TestCompare_ClassifiesDepEdgeMismatch(t *testing.T) {
	oracle := []byte(`{"id":"X-1","dependencies":["X-2"]}`)
	candidate := []byte(`{"id":"X-1","dependencies":["X-2","X-3"]}`)
	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if result.Mismatch == nil {
		t.Fatalf("expected a mismatch")
	}
	if result.Mismatch.Category != "dep-edge" {
		t.Errorf("Category = %q, want dep-edge", result.Mismatch.Category)
	}
}

func TestCompare_ClassifiesAttributionMismatch(t *testing.T) {
	oracle := []byte(`{"id":"X-1","assignee":"alice"}`)
	candidate := []byte(`{"id":"X-1","assignee":"bob"}`)
	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if result.Mismatch == nil || result.Mismatch.Category != "attribution" {
		t.Errorf("expected attribution mismatch, got %+v", result.Mismatch)
	}
}

func TestCompare_ClassifiesAsOfMismatchWhenOnlyTimestampsDiffer(t *testing.T) {
	oracle := []byte(`{"id":"X-1","updated_at":"2026-09-27T00:00:00Z"}`)
	candidate := []byte(`{"id":"X-1","updated_at":"2026-09-27T00:00:01Z"}`)
	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if result.Mismatch == nil || result.Mismatch.Category != "as-of-mismatch" {
		t.Errorf("expected as-of-mismatch, got %+v", result.Mismatch)
	}
}

func TestCompare_UnclassifiedFallback(t *testing.T) {
	oracle := []byte(`{"id":"X-1","some_unrelated_field":"a"}`)
	candidate := []byte(`{"id":"X-1","some_unrelated_field":"b"}`)
	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if result.Mismatch == nil || result.Mismatch.Category != "unclassified" {
		t.Errorf("expected unclassified, got %+v", result.Mismatch)
	}
}

func TestCanonicalize_SortsKeys(t *testing.T) {
	got, err := Canonicalize([]byte(`{"b":1,"a":2}`))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	want := `{"a":2,"b":1}`
	if string(got) != want {
		t.Errorf("Canonicalize = %s, want %s", got, want)
	}
}

func TestHash_DeterministicForEqualInput(t *testing.T) {
	canon, err := Canonicalize([]byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	h1 := Hash(canon)
	h2 := Hash(canon)
	if h1 != h2 {
		t.Errorf("Hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("Hash length = %d, want 64 (SHA-256 hex)", len(h1))
	}
}

func TestCompare_InvalidJSONReturnsError(t *testing.T) {
	_, err := Compare([]byte(`not json`), []byte(`{}`))
	if err == nil {
		t.Fatalf("expected error for invalid JSON oracle payload")
	}
}

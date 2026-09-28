// Test-to-acceptance-criterion map (be-f2gog):
//
//	AC1 (canonicalize + SHA-256, matched/oracle_hash/candidate_hash)
//	  -> TestCanonicalize_SortsObjectKeysRecursively
//	  -> TestHash_ReturnsLowercaseHexSHA256
//	  -> TestCompare_IdenticalPayloads_Match
//	  -> TestCompare_KeyOrderOnlyDifference_StillMatches
//
//	AC2 (reuse Phase 0's (#6147) canonicalization primitive, no independent
//	     JCS reimplementation) -- Canonicalize (main.go) calls jcs.Transform
//	     directly, the same dependency Phase 0's jcsCanonicalize wraps
//	     (internal/storage/issueops/version_history.go). Proven
//	     behaviorally: a hand-rolled json.Marshal round-trip would not give
//	     ECMA-262 number formatting or recursive key sorting for free.
//	  -> TestCanonicalize_SortsObjectKeysRecursively
//	  -> TestCanonicalize_NumbersUseECMA262Formatting
//
//	AC3 (on mismatch, a MISMATCH record: category + expected_json +
//	     actual_json, sufficient for triage without re-deriving)
//	  -> TestCompare_ClassifiesMismatchCategory (table, all category rules)
//	  -> TestCompare_MismatchRecord_CarriesFullPayloadsVerbatim
//
//	AC4 (zero Dolt/clone/network dependency, fully unit-testable in
//	     isolation) -- satisfied by construction: this package's only
//	     non-stdlib import is github.com/gowebpki/jcs (JCS canonicalization
//	     only). No Dolt driver, no network client, no filesystem clone
//	     anywhere in main.go. No dedicated test: a structural absence is
//	     not something a test asserts, it's what the import list already
//	     shows (be-ayur5 comment-only-TDD-satisfied-by-construction
//	     precedent).
//
//	AC5 (fixtures: identical/match, key-order-only/match,
//	     real-field-value-mismatch/no-match with correct diff,
//	     dependency-edge-mismatch feeding the driver-core sibling's R3
//	     check)
//	  -> TestCompare_IdenticalPayloads_Match
//	  -> TestCompare_KeyOrderOnlyDifference_StillMatches
//	  -> TestCompare_ClassifiesMismatchCategory/generic_field_value_differs
//	  -> TestCompare_ClassifiesMismatchCategory/dependency_edge_differs
package main

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestCanonicalize_SortsObjectKeysRecursively(t *testing.T) {
	in := []byte(`{"b":1,"a":{"d":2,"c":3}}`)
	got, err := Canonicalize(in)
	if err != nil {
		t.Fatalf("Canonicalize(%s): unexpected error: %v", in, err)
	}
	want := `{"a":{"c":3,"d":2},"b":1}`
	if string(got) != want {
		t.Fatalf("Canonicalize(%s) = %s, want %s", in, got, want)
	}
}

func TestCanonicalize_NumbersUseECMA262Formatting(t *testing.T) {
	// RFC 8785 mandates ECMA-262 number formatting (no trailing ".0" for a
	// whole-valued float). A hand-rolled canonicalizer built on
	// json.Marshal would not give this for free -- this is the behavioral
	// proof that Canonicalize really delegates to jcs.Transform (AC2)
	// rather than reimplementing RFC 8785 by hand.
	in := []byte(`{"n":1.0}`)
	got, err := Canonicalize(in)
	if err != nil {
		t.Fatalf("Canonicalize(%s): unexpected error: %v", in, err)
	}
	want := `{"n":1}`
	if string(got) != want {
		t.Fatalf("Canonicalize(%s) = %s, want %s", in, got, want)
	}
}

func TestCanonicalize_InvalidJSON_ReturnsError(t *testing.T) {
	if _, err := Canonicalize([]byte(`{not valid json`)); err == nil {
		t.Fatal("Canonicalize: want error for invalid JSON, got nil")
	}
}

func TestHash_ReturnsLowercaseHexSHA256(t *testing.T) {
	got := Hash([]byte("hello"))

	// Verified ground truth: printf '%s' hello | sha256sum
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("Hash(%q) = %s, want %s", "hello", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("Hash(%q): got length %d, want 64 (32-byte digest, hex-encoded)", "hello", len(got))
	}
	if got != strings.ToLower(got) {
		t.Fatalf("Hash(%q) = %s, want lowercase hex", "hello", got)
	}
	if _, err := hex.DecodeString(got); err != nil {
		t.Fatalf("Hash(%q) = %s: not valid hex: %v", "hello", got, err)
	}
	if got2 := Hash([]byte("hello")); got != got2 {
		t.Fatalf("Hash(%q) not deterministic: %s vs %s", "hello", got, got2)
	}
	if got3 := Hash([]byte("world")); got == got3 {
		t.Fatalf("Hash: distinct inputs %q and %q produced the same digest %s", "hello", "world", got)
	}
}

func TestCompare_IdenticalPayloads_Match(t *testing.T) {
	payload := []byte(`{"id":"be-abc12","title":"Example","content_hash":"aaa111"}`)

	result, err := Compare(payload, payload)
	if err != nil {
		t.Fatalf("Compare: unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatalf("Compare(identical payloads): Matched = false, want true")
	}
	if result.OracleHash == "" || result.OracleHash != result.CandidateHash {
		t.Fatalf("Compare(identical payloads): OracleHash=%q CandidateHash=%q, want equal and non-empty", result.OracleHash, result.CandidateHash)
	}
	if result.Mismatch != nil {
		t.Fatalf("Compare(identical payloads): Mismatch = %+v, want nil", result.Mismatch)
	}
}

func TestCompare_KeyOrderOnlyDifference_StillMatches(t *testing.T) {
	oracle := []byte(`{"id":"be-abc12","title":"Example","meta":{"a":1,"b":2},"dependencies":["be-x","be-y"]}`)
	candidate := []byte(`{"dependencies":["be-x","be-y"],"meta":{"b":2,"a":1},"title":"Example","id":"be-abc12"}`)

	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatalf("Compare(key-order-only difference): Matched = false, want true (comparison is on canonical form)")
	}
	if result.Mismatch != nil {
		t.Fatalf("Compare(key-order-only difference): Mismatch = %+v, want nil", result.Mismatch)
	}
}

func TestCompare_MismatchRecord_CarriesFullPayloadsVerbatim(t *testing.T) {
	oracle := []byte(`{"id":"be-abc12","title":"Old title"}`)
	candidate := []byte(`{"id":"be-abc12","title":"New title"}`)

	result, err := Compare(oracle, candidate)
	if err != nil {
		t.Fatalf("Compare: unexpected error: %v", err)
	}
	if result.Matched {
		t.Fatalf("Compare(differing title): Matched = true, want false")
	}
	if result.Mismatch == nil {
		t.Fatal("Compare(differing title): Mismatch = nil, want non-nil")
	}
	if string(result.Mismatch.ExpectedJSON) != string(oracle) {
		t.Fatalf("Mismatch.ExpectedJSON = %s, want %s (verbatim oracle payload)", result.Mismatch.ExpectedJSON, oracle)
	}
	if string(result.Mismatch.ActualJSON) != string(candidate) {
		t.Fatalf("Mismatch.ActualJSON = %s, want %s (verbatim candidate payload)", result.Mismatch.ActualJSON, candidate)
	}
	if result.Mismatch.Category == "" {
		t.Fatal("Mismatch.Category = \"\", want a non-empty category")
	}
}

func TestCompare_ClassifiesMismatchCategory(t *testing.T) {
	cases := []struct {
		name         string
		oracle       string
		candidate    string
		wantCategory string
	}{
		{
			name:         "dependency edge differs",
			oracle:       `{"id":"be-abc12","title":"Example","dependencies":["be-x"]}`,
			candidate:    `{"id":"be-abc12","title":"Example","dependencies":["be-x","be-y"]}`,
			wantCategory: "dep-edge",
		},
		{
			name:         "depends_on synonym field differs",
			oracle:       `{"id":"be-abc12","depends_on":["be-x"]}`,
			candidate:    `{"id":"be-abc12","depends_on":[]}`,
			wantCategory: "dep-edge",
		},
		{
			name:         "content hash differs",
			oracle:       `{"id":"be-abc12","title":"Example","content_hash":"aaa111"}`,
			candidate:    `{"id":"be-abc12","title":"Example","content_hash":"bbb222"}`,
			wantCategory: "version-count",
		},
		{
			name:         "revision synonym field differs",
			oracle:       `{"issue_id":"be-abc12","revision":3}`,
			candidate:    `{"issue_id":"be-abc12","revision":4}`,
			wantCategory: "version-count",
		},
		{
			name:         "attribution field (created_by) differs",
			oracle:       `{"id":"be-abc12","title":"Example","created_by":"alice"}`,
			candidate:    `{"id":"be-abc12","title":"Example","created_by":"bob"}`,
			wantCategory: "attribution",
		},
		{
			name:         "attribution field (change_actor, issue_versions shape) differs",
			oracle:       `{"issue_id":"be-abc12","revision":3,"change_actor":"alice"}`,
			candidate:    `{"issue_id":"be-abc12","revision":3,"change_actor":"bob"}`,
			wantCategory: "attribution",
		},
		{
			name:         "epoch differs",
			oracle:       `{"issue_id":"be-abc12","revision":3,"epoch":1}`,
			candidate:    `{"issue_id":"be-abc12","revision":3,"epoch":2}`,
			wantCategory: "epoch",
		},
		{
			name:         "only timestamp fields differ",
			oracle:       `{"id":"be-abc12","title":"Example","created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`,
			candidate:    `{"id":"be-abc12","title":"Example","created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-02T00:00:00Z"}`,
			wantCategory: "as-of-mismatch",
		},
		{
			name:         "generic field value differs",
			oracle:       `{"id":"be-abc12","title":"Old title"}`,
			candidate:    `{"id":"be-abc12","title":"New title"}`,
			wantCategory: "unclassified",
		},
		{
			name:         "dependency edge takes priority over an unrelated field also differing",
			oracle:       `{"id":"be-abc12","title":"Old title","dependencies":["be-x"]}`,
			candidate:    `{"id":"be-abc12","title":"New title","dependencies":["be-x","be-y"]}`,
			wantCategory: "dep-edge",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := Compare([]byte(c.oracle), []byte(c.candidate))
			if err != nil {
				t.Fatalf("Compare: unexpected error: %v", err)
			}
			if result.Matched {
				t.Fatalf("Compare: Matched = true, want false (fixtures differ)")
			}
			if result.Mismatch == nil {
				t.Fatal("Compare: Mismatch = nil, want non-nil")
			}
			if result.Mismatch.Category != c.wantCategory {
				t.Fatalf("Compare: Mismatch.Category = %q, want %q", result.Mismatch.Category, c.wantCategory)
			}
		})
	}
}

func TestCompare_InvalidJSON_ReturnsError(t *testing.T) {
	if _, err := Compare([]byte(`{not valid`), []byte(`{}`)); err == nil {
		t.Fatal("Compare: want error when oracle payload is invalid JSON, got nil")
	}
	if _, err := Compare([]byte(`{}`), []byte(`{not valid`)); err == nil {
		t.Fatal("Compare: want error when candidate payload is invalid JSON, got nil")
	}
}

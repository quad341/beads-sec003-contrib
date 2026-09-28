package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMismatches_ParsesJSONLInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mismatches.jsonl")
	content := `{"run_id":"run-1","source_commit":"aaa111","issue_id":"be-1","category":"attribution","expected_json":{"a":1},"actual_json":{"a":2}}
{"run_id":"run-1","source_commit":"bbb222","issue_id":"be-2","category":"epoch","expected_json":{"b":1},"actual_json":{"b":2}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadMismatches(path)
	if err != nil {
		t.Fatalf("ReadMismatches: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d mismatches, want 2", len(got))
	}
	if got[0].IssueID != "be-1" || got[1].IssueID != "be-2" {
		t.Errorf("mismatches out of order or misparsed: %+v", got)
	}
	if got[0].Category != "attribution" || got[0].SourceCommit != "aaa111" {
		t.Errorf("first mismatch fields wrong: %+v", got[0])
	}
}

func TestReadMismatches_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mismatches.jsonl")
	content := "{\"run_id\":\"run-1\",\"source_commit\":\"aaa\",\"issue_id\":\"be-1\",\"category\":\"epoch\"}\n\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadMismatches(path)
	if err != nil {
		t.Fatalf("ReadMismatches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d mismatches, want 1 (blank lines must be skipped)", len(got))
	}
}

func TestReadMismatches_MissingFileMeansNoMismatches(t *testing.T) {
	// driver-core's store only creates mismatches.jsonl on the first actual
	// WriteMismatch call (scripts/driver-core/store.go); a clean run leaves it
	// absent, and that must read as zero mismatches, not an error.
	got, err := ReadMismatches(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatalf("ReadMismatches on missing file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d mismatches, want 0", len(got))
	}
}

func TestFingerprint_StableAndDistinguishing(t *testing.T) {
	a := MismatchRecord{RunID: "run-1", SourceCommit: "aaa", IssueID: "be-1", Category: "epoch"}
	aAgain := MismatchRecord{RunID: "run-2", SourceCommit: "aaa", IssueID: "be-1", Category: "epoch"}
	b := MismatchRecord{RunID: "run-1", SourceCommit: "bbb", IssueID: "be-1", Category: "epoch"}

	if Fingerprint(a) != Fingerprint(aAgain) {
		t.Error("fingerprint must not depend on run_id: the same underlying regression recurring across nightly runs must fingerprint identically, or it will be re-filed every night")
	}
	if Fingerprint(a) == Fingerprint(b) {
		t.Error("different source commits must not collide")
	}
}

func TestFilterNew_ExcludesAlreadyFiled(t *testing.T) {
	m1 := MismatchRecord{SourceCommit: "aaa", IssueID: "be-1", Category: "epoch"}
	m2 := MismatchRecord{SourceCommit: "bbb", IssueID: "be-2", Category: "attribution"}
	filed := map[string]string{Fingerprint(m1): "be-hs42e.5.99"}

	got := FilterNew([]MismatchRecord{m1, m2}, filed)

	if len(got) != 1 || got[0].IssueID != "be-2" {
		t.Errorf("FilterNew = %+v, want only m2", got)
	}
}

func TestFilterNew_NilFiledMapMeansAllNew(t *testing.T) {
	m1 := MismatchRecord{SourceCommit: "aaa", IssueID: "be-1", Category: "epoch"}
	got := FilterNew([]MismatchRecord{m1}, nil)
	if len(got) != 1 {
		t.Errorf("FilterNew with nil filed map = %+v, want all mismatches treated as new", got)
	}
}

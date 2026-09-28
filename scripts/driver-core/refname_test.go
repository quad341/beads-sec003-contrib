package main

import (
	"regexp"
	"testing"
)

// doltHashLikePattern is the shape driver-core must never mint a run id or
// ref name matching -- a bare Dolt commit-hash-shaped string (NFR5/R2).
var doltHashLikePattern = regexp.MustCompile(`^[0-9a-v]{16,31}$`)

func TestGenerateRunID_NeverLooksLikeADoltCommitHash(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := GenerateRunID()
		if doltHashLikePattern.MatchString(id) {
			t.Fatalf("GenerateRunID() = %q, matches forbidden dolt-commit-hash-like pattern", id)
		}
	}
}

func TestGenerateRunID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := GenerateRunID()
		if seen[id] {
			t.Fatalf("GenerateRunID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestGenerateRunID_HasStablePrefix(t *testing.T) {
	id := GenerateRunID()
	if len(id) < len("replay-run-") || id[:len("replay-run-")] != "replay-run-" {
		t.Errorf("GenerateRunID() = %q, want replay-run- prefix", id)
	}
}

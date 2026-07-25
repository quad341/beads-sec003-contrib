package main

import (
	"strings"
	"testing"
)

// TestAttachNotProtoMessage is the regression test for be-j2rcl.
//
// Before the fix, `bd mol pour <proto> --attach <existing-issue-id>` refused
// with "is not a proto (missing 'template' label)" - wording indistinguishable
// from a broken detection gate. On 2026-07-24, eleven independent agent
// sessions read that refusal as "the gate is broken" and bypassed it, because
// nothing in the message said pour was structurally the wrong tool for
// attaching to an EXISTING issue.
//
// After the fix, attachNotProtoMessage must name pour as the wrong tool and
// point at the caller's attach-to-issue primitive instead.
func TestAttachNotProtoMessage(t *testing.T) {
	msg, hint := attachNotProtoMessage("ga-d8oqyt")

	wantMsg := `--attach expects a proto id, got issue id "ga-d8oqyt".`
	if msg != wantMsg {
		t.Errorf("message = %q, want %q", msg, wantMsg)
	}

	wantHintSubstrings := []string{
		"`mol pour` always creates a NEW root and cannot attach to an issue that",
		"already exists.",
		"To expand an EXISTING issue into a sub-workflow, use your",
		"workflow tool's attach-to-issue primitive instead.",
	}
	for _, want := range wantHintSubstrings {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\ngot: %q", want, hint)
		}
	}

	if strings.Contains(msg, "is not a proto (missing") || strings.Contains(hint, "is not a proto (missing") {
		t.Error("message/hint should not contain the old generic wording")
	}
}

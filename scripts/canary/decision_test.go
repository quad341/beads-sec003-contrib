package main

import (
	"strings"
	"testing"
)

func TestBuildMayorSummary_FailedRun(t *testing.T) {
	o := RunOutcome{RunID: "run-9", IntegrationSHA: "deadbeef00112233", Status: "failed"}
	subject, body := BuildMayorSummary(o)

	if !strings.Contains(strings.ToLower(subject), "fail") {
		t.Errorf("subject %q must flag the failed-to-run case (AF2 step 5: silent failure-to-run must be visible)", subject)
	}
	if !strings.Contains(body, "run-9") || !strings.Contains(body, "deadbeef") {
		t.Errorf("body %q must cite the run id and integration SHA", body)
	}
}

func TestBuildMayorSummary_CleanPass(t *testing.T) {
	o := RunOutcome{RunID: "run-10", IntegrationSHA: "cafef00d00112233", Status: "completed", TotalMismatches: 0}
	subject, body := BuildMayorSummary(o)

	if subject == "" || body == "" {
		t.Fatal("AF2 step 5: mayor must get a summary regardless of pass/fail-count, never silence on a clean pass")
	}
	if strings.Contains(strings.ToLower(subject), "regression") {
		t.Errorf("clean-pass subject %q must not read as a regression alert", subject)
	}
}

func TestBuildMayorSummary_Regression(t *testing.T) {
	mismatches := []MismatchRecord{
		{SourceCommit: "aaa", IssueID: "be-1", Category: "epoch"},
		{SourceCommit: "bbb", IssueID: "be-2", Category: "attribution"},
	}
	o := RunOutcome{
		RunID:           "run-11",
		IntegrationSHA:  "0123456789abcdef",
		Status:          "completed",
		NewMismatches:   mismatches,
		TotalMismatches: 2,
	}
	subject, body := BuildMayorSummary(o)

	if !strings.Contains(strings.ToLower(subject), "regression") {
		t.Errorf("subject %q must flag a regression", subject)
	}
	if !strings.Contains(body, "2") {
		t.Errorf("body %q must cite the new-mismatch count", body)
	}
}

func TestBuildBeadCreateArgs(t *testing.T) {
	m := MismatchRecord{
		RunID:        "run-11",
		SourceCommit: "0123456789abcdef",
		IssueID:      "be-1",
		Category:     "epoch",
		ExpectedJSON: []byte(`{"epoch":1}`),
		ActualJSON:   []byte(`{"epoch":2}`),
	}
	args := BuildBeadCreateArgs("be-hs42e.5", m)

	if len(args) == 0 || args[0] != "create" {
		t.Fatalf("args = %v, want it to start with the bd subcommand %q", args, "create")
	}
	if !containsArg(args, "--parent=be-hs42e.5") {
		t.Errorf("args %v must parent the bead to be-hs42e.5 (AF2 step 3)", args)
	}
	if !containsArg(args, "--label=bug") {
		t.Errorf("args %v must label the bead bug (UC3)", args)
	}
	if !containsPrefixed(args, "--title=", "epoch") || !containsPrefixed(args, "--title=", "be-1") {
		t.Errorf("args %v title must name the category and issue", args)
	}
	if !containsPrefixed(args, "--description=", "0123456789abcdef") ||
		!containsPrefixed(args, "--description=", `"epoch":1`) ||
		!containsPrefixed(args, "--description=", `"epoch":2`) {
		t.Errorf("args %v description must carry the full repro (FR4): commit + expected + actual", args)
	}
}

func TestBuildSlingArgs(t *testing.T) {
	args := BuildSlingArgs("beads/builder", "be-hs42e.5.100")
	want := []string{"sling", "beads/builder", "be-hs42e.5.100", "--nudge"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("0123456789abcdef"); got != "01234567" {
		t.Errorf("shortSHA(long) = %q, want %q", got, "01234567")
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("shortSHA(short) = %q, want unchanged %q", got, "abc")
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsPrefixed(args []string, prefix, substr string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) && strings.Contains(a, substr) {
			return true
		}
	}
	return false
}

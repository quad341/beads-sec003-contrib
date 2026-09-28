package main

import "fmt"

// RunOutcome is this package's own summary of one canary run, built from
// driver-core's ReplayRun/Mismatch JSONL output (never driver-core's types
// directly -- decoupled-binary design) plus this package's own UC3
// already-filed filtering.
type RunOutcome struct {
	RunID           string
	IntegrationSHA  string
	Status          string // driver-core's own vocabulary: "running" | "completed" | "failed"
	NewMismatches   []MismatchRecord
	TotalMismatches int
}

// BuildMayorSummary builds the unconditional per-run mayor summary AF2 step
// 5 requires: "mayor gets a summary regardless of pass/fail-count... silence
// must never be the only signal." Three branches, never a fourth silent one.
func BuildMayorSummary(o RunOutcome) (subject, body string) {
	switch {
	case o.Status != "completed":
		subject = fmt.Sprintf("Nightly canary FAILED TO RUN (%s)", o.RunID)
		body = fmt.Sprintf(
			"Canary run %s against integration@%s did not complete (status=%s).\nSee driver-core logs for the failure.",
			o.RunID, shortSHA(o.IntegrationSHA), o.Status)
	case len(o.NewMismatches) == 0:
		subject = fmt.Sprintf("Nightly canary clean pass (%s)", o.RunID)
		body = fmt.Sprintf(
			"Canary run %s against integration@%s completed with 0 new mismatches (%d total).",
			o.RunID, shortSHA(o.IntegrationSHA), o.TotalMismatches)
	default:
		subject = fmt.Sprintf("Nightly canary REGRESSION: %d new mismatch(es) (%s)", len(o.NewMismatches), o.RunID)
		body = fmt.Sprintf(
			"Canary run %s against integration@%s found %d new mismatch(es) (%d total). Filed one bead per mismatch, routed for build.",
			o.RunID, shortSHA(o.IntegrationSHA), len(o.NewMismatches), o.TotalMismatches)
	}
	return subject, body
}

// BuildBeadCreateArgs builds the `bd create` argv for filing one bug bead
// from a mismatch (UC3): parented under parentID, labeled bug, with the
// full repro (source commit, expected vs. actual) in the description (FR4).
func BuildBeadCreateArgs(parentID string, m MismatchRecord) []string {
	title := fmt.Sprintf("canary: %s mismatch on %s at %s", m.Category, m.IssueID, shortSHA(m.SourceCommit))
	description := fmt.Sprintf(
		"Nightly canary replay found a mismatch (category: %s) for %s.\n\nSource commit: %s\nRun: %s\n\nExpected:\n%s\n\nActual:\n%s\n",
		m.Category, m.IssueID, m.SourceCommit, m.RunID, string(m.ExpectedJSON), string(m.ActualJSON))
	return []string{
		"create",
		"--parent=" + parentID,
		"--label=bug",
		"--title=" + title,
		"--description=" + description,
		"--json",
	}
}

// BuildSlingArgs builds the `gc sling` argv that routes a newly filed
// mismatch bead the same way UC3 routes any other build-ready work.
func BuildSlingArgs(target, beadID string) []string {
	return []string{"sling", target, beadID, "--nudge"}
}

// shortSHA truncates a commit SHA to 8 characters for display, leaving
// shorter values (e.g. test fixtures) unchanged.
func shortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

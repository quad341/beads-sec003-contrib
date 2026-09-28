package scripts_test

import (
	"strings"
	"testing"
)

// TestNightlyCanaryWorkflow_RunsOnScheduleAgainstIntegrationTip covers
// be-hs42e.5.6 (UC4/AF2): the nightly canary must fire on its own cadence
// (FR7), always build/checkout integration's current tip rather than a
// pinned commit (architecture §2 -- "a canary that silently drifts to a
// stale binary would defeat its own purpose"), and go through the
// env-sanitized entry point NFR3 requires instead of a bare shell step.
func TestNightlyCanaryWorkflow_RunsOnScheduleAgainstIntegrationTip(t *testing.T) {
	workflow := readCIWorkflow(t, "nightly-canary.yml")

	if len(workflow.On.Schedule) == 0 {
		t.Fatal("nightly-canary.yml must define a schedule trigger")
	}
	for _, sched := range workflow.On.Schedule {
		if sched.Cron == "" {
			t.Error("schedule trigger must set a cron expression")
		}
	}

	job := workflow.job(t, "canary")

	var checksOutIntegrationTip bool
	for _, step := range job.Steps {
		if step.With["repository"] == "versioned-beads/beads" && step.With["ref"] == "integration" {
			checksOutIntegrationTip = true
		}
	}
	if !checksOutIntegrationTip {
		t.Error("canary job must check out versioned-beads/beads@integration: the harness always builds fresh from integration's tip, never a pinned/stale checkout")
	}

	if !containsRunSubstring(job, "scripts/ci/canary-core.sh") {
		t.Error("canary job must invoke the env-sanitized entry point scripts/ci/canary-core.sh (NFR3), not a bare shell command")
	}
}

func containsRunSubstring(job ciWorkflowJob, substr string) bool {
	for _, step := range job.Steps {
		if strings.Contains(step.Run, substr) {
			return true
		}
	}
	return false
}

package main

import "testing"

func TestSanitizedEnv_StripsAmbientVars(t *testing.T) {
	base := []string{
		"BEADS_DOLT_SERVER_PORT=28231",
		"BEADS_DOLT_PORT=28231",
		"BEADS_ACTOR=someone",
		"BD_ACTOR=someone",
		"GT_ROOT=/some/path",
		"BEADS_DIR=/some/beads/dir",
		"BEADS_HOLDER_TOKEN=secret",
		"GC_BEADS_SCOPE_ROOT=/scope",
		"BEADS_DOLT_AUTO_START=1",
		"BEADS_DOLT_SYNC_CLI_REMOTES=1",
		"BEADS_BACKUP_ENABLED=1",
		"PATH=/usr/bin",
		"HOME=/home/someone",
	}
	got := sanitizedEnv(base)

	for _, forbidden := range []string{
		"BEADS_DOLT_SERVER_PORT", "BEADS_DOLT_PORT", "BEADS_ACTOR", "BD_ACTOR", "GT_ROOT",
		"BEADS_DIR", "BEADS_HOLDER_TOKEN", "GC_BEADS_SCOPE_ROOT", "BEADS_DOLT_AUTO_START",
		"BEADS_DOLT_SYNC_CLI_REMOTES", "BEADS_BACKUP_ENABLED",
	} {
		if hasVar(got, forbidden) {
			t.Errorf("sanitizedEnv leaked ambient var %s", forbidden)
		}
	}
	for _, kept := range []string{"PATH", "HOME"} {
		if !hasVar(got, kept) {
			t.Errorf("sanitizedEnv dropped non-ambient var %s", kept)
		}
	}
}

func TestSanitizedEnv_EmptyInput(t *testing.T) {
	got := sanitizedEnv(nil)
	if len(got) != 0 {
		t.Errorf("sanitizedEnv(nil) = %v, want empty", got)
	}
}

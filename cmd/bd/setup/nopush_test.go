package setup

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/templates/agents"
)

// stubDetectRenderOptsNoPush overrides detectRenderOptsImpl to return opts
// with NoPush set to the given value, so tests control the render path
// without touching viper config.
func stubDetectRenderOptsNoPush(t *testing.T, noPush bool) {
	t.Helper()
	orig := detectRenderOptsImpl
	detectRenderOptsImpl = func() agents.RenderOpts {
		return agents.RenderOpts{HasRemote: true, NoPush: noPush}
	}
	t.Cleanup(func() { detectRenderOptsImpl = orig })
}

// TestInstallAgentsNoPushTrue_OmitsDoltPush verifies that when the rig is
// configured with no-push:true, installAgents writes an AGENTS.md that
// contains no 'bd dolt push' reference.
func TestInstallAgentsNoPushTrue_OmitsDoltPush(t *testing.T) {
	stubDetectRenderOptsNoPush(t, true)

	env, _, _ := newFactoryTestEnv(t)
	integration := agentsIntegration{
		name:         "TestAgent",
		setupCommand: "bd setup testagent",
		profile:      agents.ProfileFull,
	}
	if err := installAgents(env, integration); err != nil {
		t.Fatalf("installAgents returned error: %v", err)
	}

	data, err := os.ReadFile(env.agentsPath)
	if err != nil {
		t.Fatalf("failed to read AGENTS.md: %v", err)
	}
	if strings.Contains(string(data), "bd dolt push") {
		t.Error("AGENTS.md written with NoPush=true should not contain 'bd dolt push'")
	}
}

// TestCheckAgentsDetectsStaleOnNoPushChange verifies that bd doctor --check=agents
// reports a stale section when the no-push value changes after initial install.
// Specifically: a file installed without no-push (HasRemote=true, NoPush=false)
// must be reported as stale when checked with NoPush=true.
func TestCheckAgentsDetectsStaleOnNoPushChange(t *testing.T) {
	// Phase 1: install with NoPush=false
	stubDetectRenderOptsNoPush(t, false)

	env, stdout, _ := newFactoryTestEnv(t)
	integration := agentsIntegration{
		name:         "TestAgent",
		setupCommand: "bd setup testagent",
		profile:      agents.ProfileFull,
	}
	if err := installAgents(env, integration); err != nil {
		t.Fatalf("installAgents (NoPush=false) returned error: %v", err)
	}

	// Phase 2: check with NoPush=true — hash differs, section is stale
	stubDetectRenderOptsNoPush(t, true)
	stdout.Reset()

	err := checkAgents(env, integration)
	if !errors.Is(err, errBeadsSectionStale) {
		t.Fatalf("expected errBeadsSectionStale after NoPush change, got %v", err)
	}
	if !strings.Contains(stdout.String(), "stale") {
		t.Error("expected 'stale' in checkAgents output after NoPush change")
	}
}

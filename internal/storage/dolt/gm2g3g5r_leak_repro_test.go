package dolt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gm-2g3g5r repro. A gc-managed city runs its production Dolt server on a
// DYNAMICALLY ALLOCATED port (28231 on this host), recorded in
// .beads/dolt-server.port and exported into every agent session as
// BEADS_DOLT_SERVER_PORT. Every beads TestMain sets BEADS_TEST_SERVER=1.
//
// prodPort below stands in for that dynamic port. It is never dialed: both
// tests call applyConfigDefaults / productionPortReasons only, which read
// env + files and open no sockets.
const reproProdPort = "59999"

// TestApplyConfigDefaults_TestModeBlocksNonDefaultProductionPort documents a
// residual gap, not the gm-2g3g5r leak itself: the leak's actual root cause
// (EnsureDoltContainerForTestMain failing open on the ambient port) is fixed
// and covered by TestEnsureDoltContainerForTestMain_NeutralizesAmbientPortOnFailure
// in internal/testutil. This test injects a production port directly via env
// var to verify Rule 3 (dolt-server.port file) still catches it: an
// operator's BEADS_TEST_SERVER=1 opt-in must fail closed if the resolved
// port genuinely matches a known production port, even though DefaultSQLPort
// (Rule 1) isn't involved. See productionPortReasons for why
// BEADS_TEST_SERVER=1 no longer suppresses Rules 2/3.
func TestApplyConfigDefaults_TestModeBlocksNonDefaultProductionPort(t *testing.T) {
	beadsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(beadsDir, "dolt-server.port"),
		[]byte(reproProdPort), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEADS_TEST_MODE", "1")
	t.Setenv("BEADS_TEST_SERVER", "1")
	t.Setenv("BEADS_DOLT_SERVER_PORT", reproProdPort)
	t.Setenv("BEADS_DOLT_PORT", "")

	cfg := &Config{Path: filepath.Join(beadsDir, "dolt"), BeadsDir: beadsDir}
	applyConfigDefaults(cfg)

	if !strings.HasPrefix(cfg.Database, "testdb_") {
		t.Fatalf("precondition: want a derived testdb_ name, got %q", cfg.Database)
	}
	if cfg.ServerPort != 1 {
		t.Errorf("LEAK: BEADS_TEST_MODE=1 resolved to production port %d "+
			"(dolt-server.port says %s) and would create %q there; want port 1",
			cfg.ServerPort, reproProdPort, cfg.Database)
	}
}

// TestProductionPortReasons_CatchesBeadsDirlessViaProductionPortEnv verifies
// Rule 2 (BEADS_PRODUCTION_PORT) catches a production port even when
// cfg.BeadsDir is empty -- the shape beads.Open (beads.go:277) actually
// produces, since it never threads a BeadsDir through. Rule 3 (the
// dolt-server.port file) is gated on cfg.BeadsDir != "" and deliberately
// does not fall back to filepath.Dir(cfg.Path) (store.go:119-126: test
// fixtures commonly set Path under /tmp with no real BeadsDir, and deriving
// one would false-positive on stray port files from leaked dev servers) --
// so a BeadsDir-less config depends on Rule 2 alone. BEADS_PRODUCTION_PORT is
// populated for real by neutralizeAmbientDoltPort on the container-failure
// path (internal/testutil/testdoltserver.go); this test sets it directly to
// isolate productionPortReasons from that plumbing.
func TestProductionPortReasons_CatchesBeadsDirlessViaProductionPortEnv(t *testing.T) {
	beadsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(beadsDir, "dolt-server.port"),
		[]byte(reproProdPort), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEADS_PRODUCTION_PORT", reproProdPort)

	// Exactly what beads.Open passes: Path set, BeadsDir empty.
	cfg := &Config{Path: filepath.Join(beadsDir, "dolt"), ServerPort: 59999}
	reasons := productionPortReasons(cfg)
	if len(reasons) == 0 {
		t.Errorf("want Rule 2 (BEADS_PRODUCTION_PORT) to catch port %d via env "+
			"var even with BeadsDir empty; got no reasons", cfg.ServerPort)
	}
}

// TestApplyConfigDefaults_NoResolvablePortFailsClosed closes the loop on the
// proposed fix: once the harness neutralizes the ambient port vars, nothing
// resolves a port, and the existing BEADS_TEST_MODE guard's `ServerPort == 0`
// branch (store.go:1504-1508) forces port 1. This already passes today --- it
// is the invariant the fix relies on, asserted so a future change cannot
// remove it silently.
func TestApplyConfigDefaults_NoResolvablePortFailsClosed(t *testing.T) {
	beadsDir := t.TempDir() // no dolt-server.port, no config.yaml
	t.Setenv("BEADS_TEST_MODE", "1")
	t.Setenv("BEADS_TEST_SERVER", "1")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")
	t.Setenv("BEADS_DOLT_PORT", "")

	cfg := &Config{Path: filepath.Join(beadsDir, "dolt"), BeadsDir: beadsDir}
	applyConfigDefaults(cfg)

	if cfg.ServerPort != 1 {
		t.Errorf("want port 1 (fail closed) with no resolvable port under "+
			"BEADS_TEST_MODE=1, got %d", cfg.ServerPort)
	}
}

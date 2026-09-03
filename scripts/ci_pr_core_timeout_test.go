package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPRCoreGoTestHasExplicitTimeout guards the failure mode already fixed
// once in this repo for main.yml's ubuntu/macOS legs (wy-5b5fbl, see
// TestMacOSTestJobsReuseWorkspaceBDBinary): go test's 10-minute per-package
// default leaves no margin for cmd/bd, so an unrelated PR (#6001, an
// engdocs-only diff) failed on a package-wide timeout naming whatever test
// happened to be running as an arbitrary "victim"
// (TestServeAnswersFromARegisteredBackendStore, which passes in isolation in
// 25.97s; the package's own CI run recorded
// "FAIL github.com/steveyegge/beads/cmd/bd 602.085s", past the 10m ceiling).
//
// pr-core.sh runs the same -race -short -skip '^TestEmbedded' ./... workload
// as those main.yml legs but without their explicit -timeout=25m -- give it
// the same margin instead of leaving it on go test's 10m default.
func TestPRCoreGoTestHasExplicitTimeout(t *testing.T) {
	const wantTimeout = "-timeout=25m"

	root := sourceRepoRoot(t)
	script, err := os.ReadFile(filepath.Join(root, "scripts", "ci", "pr-core.sh"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(script), wantTimeout) {
		t.Errorf("scripts/ci/pr-core.sh go test invocation does not contain %q; "+
			"go test's 10m per-package default leaves cmd/bd no margin above its "+
			"measured ~600s near-saturation (see main.yml's -timeout=25m siblings "+
			"for the same -race -short -skip '^TestEmbedded' ./... workload)", wantTimeout)
	}
}

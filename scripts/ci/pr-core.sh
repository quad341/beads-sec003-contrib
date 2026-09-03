#!/usr/bin/env bash
# Required fast PR Go test contract.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=../.buildflags
source "$REPO_ROOT/.buildflags"
# shellcheck source=lib/timing.sh
source "$REPO_ROOT/scripts/ci/lib/timing.sh"
# shellcheck source=lib/test-env.sh
source "$REPO_ROOT/scripts/ci/lib/test-env.sh"

cd "$REPO_ROOT"

beads_test_env_enter

GO_TEST_PKG_PARALLEL="${GO_TEST_PKG_PARALLEL:-4}"
GO_TEST_PARALLEL="${GO_TEST_PARALLEL:-4}"

# -timeout is explicit because go test's 10m per-package default leaves
# cmd/bd no margin above its measured near-saturation, so an unrelated PR
# can fail on a package-wide timeout naming an arbitrary victim test
# (gastownhall/beads#6001). Matches the -timeout=25m already used for this
# same -race -short -skip '^TestEmbedded' ./... workload in main.yml's
# ubuntu/macOS legs.
ci_time "pr-core go test" -- \
    go test -p "$GO_TEST_PKG_PARALLEL" -parallel "$GO_TEST_PARALLEL" -race -short -timeout=25m -skip '^TestEmbedded' ./...

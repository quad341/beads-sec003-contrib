#!/usr/bin/env bash
# Nightly canary entry point (be-hs42e.5.6, UC4/AF2/NFR3): builds
# scripts/canary fresh from this checkout and runs it against the corpus and
# integration checkout the workflow provides.
#
# Unlike scripts/ci/pr-core.sh, this does not call beads_test_env_enter:
# that helper isolates a *test* run from any real bd database, but canary's
# own bd/gc calls (filing mismatch beads, mailing the mayor) must reach the
# real project database and fleet, not a throwaway one. The ambient-var
# stripping NFR3 requires for the corpus-acquire/driver-core subprocesses
# canary spawns lives in Go (scripts/canary's own sanitizedEnv, mirroring
# scripts/driver-core/env.go), next to the exec.Command calls it protects,
# not here.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_ROOT/.buildflags"
cd "$REPO_ROOT"

: "${CANARY_INTEGRATION_REPO:?CANARY_INTEGRATION_REPO must point at the checked-out versioned-beads/beads@integration working copy}"
: "${CANARY_CORPUS_DATA_DIR:?CANARY_CORPUS_DATA_DIR must point at the shared Dolt server data_dir containing the production corpus}"
: "${CANARY_WORK_DIR:=$(mktemp -d)}"
: "${CANARY_CORPUS:=gascity}"
: "${CANARY_SAMPLE_SIZE:=200}"
: "${CANARY_ROOT_BEAD:=be-hs42e.5}"
: "${CANARY_SLING_TARGET:=beads/builder}"

bin_dir="$(mktemp -d)"
go build -o "$bin_dir/canary" ./scripts/canary

exec "$bin_dir/canary" \
    -harness-repo "$REPO_ROOT" \
    -integration-repo "$CANARY_INTEGRATION_REPO" \
    -corpus-data-dir "$CANARY_CORPUS_DATA_DIR" \
    -corpus "$CANARY_CORPUS" \
    -work-dir "$CANARY_WORK_DIR" \
    -sample-size "$CANARY_SAMPLE_SIZE" \
    -root-bead "$CANARY_ROOT_BEAD" \
    -sling-target "$CANARY_SLING_TARGET"

# bd-lfak: bd preflight --fix mode (Phase 3)

**Date:** 2026-05-20  
**PM:** beads/pm  
**Epic:** bd-lfak — bd preflight: PR readiness checks for contributors

## Status (updated 2026-05-21 session 13)

Phases 1 (static checklist) and 2 (--check automated checks) are **complete** — implemented via commits bd-lfak.3-bd-lfak.5 (lint, nix-hash staleness, version sync). The core success metrics are substantially met.

Phase 3 (`--fix` mode): **All implementation and test beads are CLOSED.** B1 (be-xra8) + B2 (be-roho) implementation in PR #4054 — review PASSED (CI 41/41, 2 non-blocking LOWs). **PR #4054 is OPEN, awaiting merge.** Deployer batch-processed all 5 queued PRs in session 4 (be-dknn, be-rqkw, be-i88i, be-ivuh, be-vd2t) — all gate PASS, mailed mayor. PRs #4022/#4028/#4053/#4054/#4055 ready to merge (no deployer write access to gastownhall/beads — human merge required). T1 (be-b6m9) tests complete on branch `tests/be-b6m9-preflight-fix` (pushed to fork). be-fe4y (submit test PR) open on beads/builder, blocked on #4054 merge.

**Session 9 note (2026-05-21):** Confirmed all 5 PRs are `mergeStateStatus=CLEAN` (no conflicts, all CI green). maintainer-pr-review skill refused: `gastownhall/beads` not in maintained-repos scope. Mailed mayor with two options: (1) merge PRs manually, or (2) authorize `GC_MPR_ALLOW_UNMAINTAINED=1` so skill can handle it. Stall now spans 5 consecutive sessions (5-9).

**Session 10 note (2026-05-21):** No change. All 5 PRs confirmed CLEAN, MERGEABLE, CI 100% (41/41 or 40/40). Stall spans 6 consecutive sessions (5-10). No operator response to mayor mail. Human merge still required.

**Session 11 note (2026-05-21):** No change. All 5 PRs confirmed CLEAN, MERGEABLE, CI 100% (41/41). Stall spans 7 consecutive sessions (5-11). No operator response. Human merge still required.

**Session 12 note (2026-05-21):** No change. All 5 PRs confirmed CLEAN (mergeStateStatus=CLEAN), CI 41/41 passed. Stall spans 8 consecutive sessions (5-12). Human merge still required.

**Session 13 note (2026-05-21):** No change. All 5 PRs confirmed CLEAN, CI 41/41. Stall spans 9 consecutive sessions (5-13). Human merge still required.

**Session 14 note (2026-05-21):** No change. All 5 PRs confirmed CLEAN (mergeStateStatus=CLEAN), CI 41/41. Stall spans 10 consecutive sessions (5-14). Human merge still required.

**Session 15 note (2026-05-21):** No change. All 5 PRs confirmed CLEAN (mergeStateStatus=CLEAN), CI 41/41. Stall spans 11 consecutive sessions (5-15). `bd ready` broken locally (be-zbxe: schema 0.48.0 vs CLI 1.0.4 column mismatch), unrelated to PRs. Human merge still required.

Remaining: (1) Human merge PR #4054 (and #4022/#4028/#4053/#4055) to gastownhall/beads main. (2) Rebase + submit test PR from tests/be-b6m9-preflight-fix (be-fe4y → beads/builder). (3) Close epic bd-lfak once test PR merges.

Phase 4 (configuration/.beads/preflight.yaml) is deferred — not in scope for this decomposition.

## Work Packages

### B1: vendorHash auto-fix — `be-xra8` ✓ CLOSED

Implement `fixNixHash()` in `cmd/bd/preflight.go`:
- Reuse `runNixHashCheck()` detection logic
- Run `nix-prefetch` / `nix hash path` to compute new vendorHash
- Update `default.nix` in-place
- Print before/after hash
- If nix tooling absent: error + exit 1

**P3, no blockers.**

### B2: version-sync auto-fix — `be-roho` ✓ CLOSED

Implement `fixVersionSync()` in `cmd/bd/preflight.go`:
- Reuse `runVersionSyncCheck()` detection logic
- Source of truth: `default.nix` version field
- Update `version.go` to match
- Print what changed

Also: remove outdated stub message (`"See bd-lfak.3 through bd-lfak.5 for implementation roadmap."`) — those were the check features, not fix features.

**P3, no blockers. Parallel with B1.**

### T1: --fix mode tests — `be-b6m9` ✓ CLOSED (beads/validator)

Add to `cmd/bd/preflight_test.go`:
- `TestPreflightFix_NixHash` — mock stale go.sum, verify default.nix updated
- `TestPreflightFix_NixHash_NixNotAvailable` — nix absent → exits 1
- `TestPreflightFix_VersionSync` — out-of-sync version.go → verify updated
- `TestPreflightFix_NothingToFix` — clean state → exits 0, no changes
- `TestPreflightFix_JSON` — --fix --json output is valid JSON

**P3, blocked by B1 + B2. 14 unit tests on branch `tests/be-b6m9-preflight-fix`, covers all fixNixHash/fixVersionSync/runFixes branches.**

### T2: submit test PR — `be-fe4y` ○ OPEN (beads/builder)

After PR #4054 merges: rebase `tests/be-b6m9-preflight-fix` onto main, open PR to gastownhall/beads. CI must pass. Test-only diff once rebase removes the implementation commits.

**P3, discovered-from be-b6m9.**

## Dependency Graph

```
be-xra8 (B1) ──┐
                ├──► be-b6m9 (T1) ──► be-fe4y (T2) ──► [epic bd-lfak closes when T2 merges]
be-roho (B2) ──┘

PR #4054 (open) → merge unblocks T2 rebase
```

## Routing

| Bead | Target | Label |
|------|--------|-------|
| be-xra8 | beads/builder | ready-to-build |
| be-roho | beads/builder | ready-to-build |
| be-b6m9 | beads/validator | needs-tests |
| be-fe4y | beads/builder | ready-to-build |

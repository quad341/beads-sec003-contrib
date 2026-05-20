# bd-lfak: bd preflight --fix mode (Phase 3)

**Date:** 2026-05-20  
**PM:** beads/pm  
**Epic:** bd-lfak — bd preflight: PR readiness checks for contributors

## Status

Phases 1 (static checklist) and 2 (--check automated checks) are **complete** — implemented via commits bd-lfak.3-bd-lfak.5 (lint, nix-hash staleness, version sync). The core success metrics are substantially met.

Phase 3 (`--fix` mode) remains: the flag is stubbed in `cmd/bd/preflight.go` with a placeholder message.

Phase 4 (configuration/.beads/preflight.yaml) is deferred — not in scope for this decomposition.

## Work Packages

### B1: vendorHash auto-fix — `be-xra8` (ready-to-build → beads/builder)

Implement `fixNixHash()` in `cmd/bd/preflight.go`:
- Reuse `runNixHashCheck()` detection logic
- Run `nix-prefetch` / `nix hash path` to compute new vendorHash
- Update `default.nix` in-place
- Print before/after hash
- If nix tooling absent: error + exit 1

**P3, no blockers.**

### B2: version-sync auto-fix — `be-roho` (ready-to-build → beads/builder)

Implement `fixVersionSync()` in `cmd/bd/preflight.go`:
- Reuse `runVersionSyncCheck()` detection logic
- Source of truth: `default.nix` version field
- Update `version.go` to match
- Print what changed

Also: remove outdated stub message (`"See bd-lfak.3 through bd-lfak.5 for implementation roadmap."`) — those were the check features, not fix features.

**P3, no blockers. Parallel with B1.**

### T1: --fix mode tests — `be-b6m9` (needs-tests → beads/validator)

Add to `cmd/bd/preflight_test.go`:
- `TestPreflightFix_NixHash` — mock stale go.sum, verify default.nix updated
- `TestPreflightFix_NixHash_NixNotAvailable` — nix absent → exits 1
- `TestPreflightFix_VersionSync` — out-of-sync version.go → verify updated
- `TestPreflightFix_NothingToFix` — clean state → exits 0, no changes
- `TestPreflightFix_JSON` — --fix --json output is valid JSON

**P3, blocked by B1 + B2.**

## Dependency Graph

```
be-xra8 (B1) ──┐
                ├──► be-b6m9 (T1) ──► [epic bd-lfak closes when T1 closes]
be-roho (B2) ──┘
```

## Routing

| Bead | Target | Label |
|------|--------|-------|
| be-xra8 | beads/builder | ready-to-build |
| be-roho | beads/builder | ready-to-build |
| be-b6m9 | beads/validator | needs-tests |

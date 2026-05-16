# Plan — New `bd backend status` command (be-8f8esf)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-8f8esf (designer, 2026-05-15)
**Parent architecture:** be-pbnoaa
**Foundation:** be-brdzcs (CLOSED — ResolveBackendInfo and dsn.RenderRedacted available)

## Goal

Add `bd backend status` as a new generic backend-health command that works across all backend
types (dolt-embedded, dolt-server, postgres). This is the canonical health check for automation
(`gc rig status`, monitoring scripts, CI) that replaces reflexive use of `bd dolt status` in
Postgres workspaces.

## Decomposition decision: one builder bead

New command = new file (`cmd/bd/backend.go` or `cmd/bd/backend_status.go`) + five tests.
Self-contained. One PR.

Builder bead: be-3nj8kh.

## Command hierarchy

```
bd backend            # parent, no-op, shows help
  bd backend status   # text + --json, exit 0/1
```

## Key design constraints

- JSON `"error"` field always present (empty string on success) — enables stable parsing by `gc rig status`.
- `"legacy_dolt_dir"` always present — schema consistency across backends.
- Exit 1 for unhealthy/unknown; exit 0 for healthy OR legacy+healthy (legacy is informational).
- Password never in any output path (uses `dsn.RenderRedacted`).
- `bd dolt --help` must reference `bd backend status` as the generic alternative.
- `bd doctor` output must include a one-line pointer to `bd backend status`.

## Visual design

Icons: `✓` (`ui.PassStyle`), `●` (`ui.FailStyle`), `⚠` (`ui.WarnStyle`). No emoji blobs.
Backend type word: `ui.AccentStyle`. Labels: `ui.MutedStyle`.

## Acceptance (builder bead be-3nj8kh)

- All states handled per design §3 (7 text states) and §4 (3 JSON shapes).
- All 5 tests pass.
- Discoverability text in `bd backend --help`, `bd dolt --help`, and `bd doctor` output.
- Exit code contract honored.
- `TestBackendStatus_NoPasswordLeak` passes.

## Routing

- Builder bead: be-3nj8kh → `beads/builder`
- Design bead be-8f8esf: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

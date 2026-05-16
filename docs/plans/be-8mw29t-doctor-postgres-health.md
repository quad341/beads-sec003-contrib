# Plan — bd doctor Postgres health + legacy detection + JSON backend block (be-8mw29t)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-8mw29t (designer, 2026-05-15)
**Parent architecture:** be-pbnoaa
**Foundation:** be-brdzcs (CLOSED)

## Goal

Extend `bd doctor` to run Postgres-specific health checks when backend is Postgres, skip Dolt-only
checks, detect legacy `.beads/dolt` artifacts (INFO, non-fatal), and always emit a structured
`backend` JSON block in `bd doctor --json` for consistent parsing by `gc rig status`.

## Decomposition decision: one builder bead

The three work streams (health checks, legacy detection, JSON block) all live in
`cmd/bd/doctor.go` + a new `internal/doctor/` file. Test fixtures are shared. One PR.

Builder bead: be-nzo2iu.

## PM decision: ℹ for INFO severity

Design §4 asked PM to confirm the icon for advisory/INFO-level messages. **Decision: `ℹ`
(U+2139 INFORMATION SOURCE), styled with `ui.WarnStyle` (yellow).** The word "legacy" always
follows the icon, so color is not the sole indicator. This satisfies the accessibility audit.
This is NOT `~` (tilde) — the ℹ is the approved choice.

## Check pipeline (from design §2)

```
if backend == postgres:
    run CheckPostgresHealth           (new)
    skip Dolt-only checks             (runServerHealth, CheckDoltStatus unchanged but skipped)
else (dolt):
    run existing Dolt checks unchanged

always:
    run CheckLegacyDoltArtifacts      (new, INFO only — never affects exit code)
```

Legacy check is INFORMATIONAL. `bd doctor && deploy` must not break for Postgres workspaces with
a `.beads/dolt` artifact.

## CRITICAL coordination — open PRs touching doctor.go

Builder MUST check PR status before starting and rebase accordingly:
- **PR#3758** — `fix(doctor): support embedded mode for safe checks` — touches `cmd/bd/doctor.go`,
  `cmd/bd/doctor_conventions.go`. New Postgres branch must merge cleanly.
- **PR#3755** — `fix(doctor): fresh-clone false positive in dolt server mode` — touches
  `cmd/bd/doctor/fresh_clone_server.go`, `legacy.go`. New `CheckLegacyDoltArtifacts` should NOT
  conflict with PR#3755's `legacy.go`. **Builder guidance:** If PR#3755 `legacy.go` is narrow
  (fresh-clone only), name new file `legacy_dolt_artifacts.go`. If broad, integrate as an
  additional check function.

## JSON backend block

Always present in `bd doctor --json`. Shape must match design §5 for both Postgres and Dolt backends.
This is the stable API surface for `gc rig status` — removals are breaking changes.

## Acceptance (builder bead be-nzo2iu)

- All 8 tests pass (see be-8mw29t §10 for list).
- Dolt workspace: `bd doctor` unchanged (regression test passes).
- Postgres workspace: CheckPostgresHealth runs; Dolt checks skipped.
- Both backends: CheckLegacyDoltArtifacts runs, INFO only, exit 0 when legacy present.
- `bd doctor --json` has top-level `backend` block always.
- Project_id drift message includes both IDs, action, `.beads` path.
- Legacy check includes copy-pasteable `mv` command with `YYYY-MM-DD` date.
- `bd doctor` runtime < 1s on happy path.
- `TestDoctor_NoPasswordLeak` passes.
- Does NOT auto-delete or modify any files.

## Routing

- Builder bead: be-nzo2iu → `beads/builder`
- Design bead be-8mw29t: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

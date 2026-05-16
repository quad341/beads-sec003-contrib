# Plan — Wire bd context/info/where + bd dolt redirect to BackendInfo resolver (be-hjsr03)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-hjsr03 (designer, 2026-05-15)
**Parent architecture:** be-pbnoaa
**Foundation:** be-brdzcs (CLOSED — ResolveBackendInfo and dsn.RenderRedacted available)

## Goal

Wire four existing CLI surfaces to the `configfile.ResolveBackendInfo` resolver so they
truthfully report the active backend instead of hardcoding dolt/direct. This is the user-visible
surface work for the pg-backend reporting feature (be-pbnoaa).

## Surfaces in scope

1. **`bd context`** (text + `--json`) — show resolved backend state for all 3 backends + 2 error states
2. **`bd info`** — drop hardcoded `"mode": "direct"`; emit resolved mode
3. **`bd where`** — add `Data lives in:` line with redacted URI per backend
4. **`bd dolt status` / `bd dolt show`** — exit 0 with redirect message when backend ≠ dolt

## Decomposition decision: one builder bead

All four surfaces share the same resolver call and the same test helper pattern
(`TestNoPasswordLeak`, backend matrix tests). Splitting would create four micro-PRs that all
import the same new helper and touch related test fixtures — no parallelization win.

One builder bead (be-0w5z7u), one PR.

## PM decisions

No open designer escalations on this bead. Design spec is complete and unambiguous.

### Visual design system

Design §3.2 specifies lipgloss styles:
- Backend type word: `ui.AccentStyle`
- `legacy` label: `ui.WarnStyle`; path after legacy: `ui.MutedStyle`
- `unknown`: `ui.FailStyle`
- Standard labels (project, prefix, actor): `ui.MutedStyle`
- `Data lives in:` label: `ui.AccentStyle`; parenthetical backend name: `ui.MutedStyle`

### dolt redirect exit code

Exit 0. This is a redirect, not an error. Scripts that call `bd dolt status` must not break
for Postgres workspaces. JSON: `{"redirected": true, "reason": "backend is postgres",
"alternatives": ["bd backend status", "bd context"]}`.

## Acceptance (builder bead be-0w5z7u)

- `bd context` (text + JSON) reflects resolved backend for all 3 states + error states.
- `bd context --json` backward-compatible superset of golden fixture (`TestBdContextJSON_Compatibility`).
- `bd info` drops `"mode": "direct"`; emits resolved mode.
- `bd where` has `Data lives in:` line correct for all 3 backends. Password never in URI.
- `bd dolt status` / `bd dolt show` exit 0 with redirect when backend ≠ dolt.
- 4 × `TestNoPasswordLeak` passes.
- `TestContextBackendField_BackendMatrix`, `TestInfoMode_BackendMatrix`, `TestWhere_DataLivesIn`,
  `TestDoltStatus_RedirectsForPostgres` all pass.

## Routing

- Builder bead: be-0w5z7u → `beads/builder`
- Design bead be-hjsr03: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

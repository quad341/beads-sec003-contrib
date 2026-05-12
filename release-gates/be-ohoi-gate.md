# Release Gate — be-ohoi (DEFER per PG-backend ship gate)

**Disposition:** DEFER. No PR opened. No PM escalation.
**Authority:** memory `deployer-pg-area-defer-no-pm-escalation`
(PM-ratified gm-b01bf6 2026-05-12). Ship-gate doc:
`docs/plans/pg-backend-ship-gate.md`.

## Bead

- ID: be-ohoi — Review: be-xush docs/POSTGRES-BACKEND.md §5 §7 §11 from outlines
- Source commit: `d312484c3`
- Source branch: `fix/be-0d4-postgres-init-guards`
- Reviewer mail: gm-d513ay (PASS verdict)

## Gate criteria

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Reviewer PASS present | PASS (gm-d513ay, in bead notes) |
| 2 | Acceptance criteria met | PASS (verified by reviewer) |
| 3 | Cherry-pick onto be-76gb stacked atop origin/main | CLEAN (`a9542e879`, +135/-9, fills §5/§7/§11 placeholders) |
| 4 | Premise present on origin/main | **FAIL** — same PG-derivative reason as be-8qo7 |

## Why DEFER

- Stacked on be-76gb (which stacks on be-8qo7) — neither ancestor
  ships under the current ship gate.
- The prose cites internal/storage/postgres symbols (DSN pool
  params at `open.go`, `_project_id` reuse at `init_postgres.go:103`,
  `sslmode` round-trip at `dsn/strip.go`) whose source files are
  absent from `origin/main`.
- Per ship gate: "any file absent from origin/main → DEFER without
  PM escalation."

## Disposition

- Bead closed with reason: `deferred per ship gate, see docs/plans/pg-backend-ship-gate.md`.
- Bead notes carry reviewer analysis. Preserved for rollup ship.

## PM notification

One-line queue entry mailed to beads/pm (see mail thread).

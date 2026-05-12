# Release Gate — be-8qo7 (DEFER per PG-backend ship gate)

**Disposition:** DEFER. No PR opened. No PM escalation.
**Authority:** memory `deployer-pg-area-defer-no-pm-escalation`
(PM-ratified gm-b01bf6 2026-05-12). Ship-gate doc:
`docs/plans/pg-backend-ship-gate.md`.

## Bead

- ID: be-8qo7 — Review: be-5954 docs/POSTGRES-BACKEND.md skeleton + §2 §6 verbatim
- Source commit: `f9833070b`
- Source branch: `fix/be-0d4-postgres-init-guards`
- Reviewer mail: gm-9rbahx (PASS verdict)

## Gate criteria

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Reviewer PASS present | PASS (gm-9rbahx, in bead notes) |
| 2 | Acceptance criteria met | PASS (verified by reviewer) |
| 3 | Cherry-pick to fresh branch off origin/main | CLEAN (`a23228f8d`, +229 lines, new file) |
| 4 | Premise present on origin/main | **FAIL** — see below |

## Why DEFER despite clean cherry-pick

- `docs/POSTGRES-BACKEND.md` is absent from `origin/main`.
- The doc describes Postgres-backend code (`bd init --backend=postgres`,
  DSN params, `internal/storage/postgres/...` symbols) whose
  implementation is NOT on `origin/main`. The branch carries ~30 PG-area
  commits (be-6fk.1 .. be-6fk.7, be-2oq, be-b8p, be-b0h, be-ry0,
  be-rhtega, be-xz4, be-y8g, be-g4x0eq, be-0d4, etc.) — none on main.
- Shipping the doc alone would describe non-existent user-facing
  functionality. Per ship gate: "any file absent from origin/main →
  DEFER without PM escalation."

## Disposition

- Bead closed with reason: `deferred per ship gate, see docs/plans/pg-backend-ship-gate.md`.
- Bead notes carry the analysis. Reviewer PASS preserved for the
  eventual rollup ship.
- Re-deploy is trivial when the ship gate lifts: same commit will
  cherry-pick clean to the rollup branch.

## PM notification

One-line queue entry mailed to beads/pm (see mail thread).

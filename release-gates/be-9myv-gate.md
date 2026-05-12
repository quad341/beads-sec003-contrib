# Release Gate — be-9myv (DEFER per PG-backend ship gate)

**Disposition:** DEFER. No PR opened. No PM escalation.
**Authority:** memory `deployer-pg-area-defer-no-pm-escalation`
(PM-ratified gm-b01bf6 2026-05-12). Ship-gate doc:
`docs/plans/pg-backend-ship-gate.md`.

## Bead

- ID: be-9myv — Review: be-jhho docs/POSTGRES-BACKEND.md §8 §9 §10 §12 from outlines (skeleton complete)
- Source commit: `62a2a5d2f` (F1 fix; re-review of be-jhho parent `d5dc18b3f`)
- Source branch: `fix/be-0d4-postgres-init-guards`
- Reviewer mail: gm-7zw4r1 (re-review PASS — single-paragraph §9 rewrite, 8+/6- on docs/POSTGRES-BACKEND.md only)

## Gate criteria

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Reviewer PASS present | PASS (gm-7zw4r1, full verdict in bead notes) |
| 2 | Acceptance criteria met | PASS (F1 fix verified against `0001_initial.up.sql:300-303` per-prefix PK and `idgen.go:18-44` mode dispatch) |
| 3 | Cherry-pick to fresh branch off origin/main | **FAIL** — modify/delete conflict |
| 4 | Premise present on origin/main | **FAIL** — see below |

## Cherry-pick verification

Cherry-picked `62a2a5d2f` onto a fresh branch off `origin/main`
(`da73b7511`):

```
CONFLICT (modify/delete): docs/POSTGRES-BACKEND.md deleted in HEAD and
modified in 62a2a5d2f (docs(postgres): be-9myv F1 fix §9 Concurrency
per-prefix counter scope).
```

The §9 fix modifies `docs/POSTGRES-BACKEND.md`, but the file does not
exist on origin/main — it is introduced by be-5954 (f9833070b, deferred
per be-8qo7 gate) and filled by siblings be-9e34 (25241ae80, deferred
per be-76gb gate), be-xush (d312484c3, deferred per be-ohoi gate),
be-jhho (d5dc18b3f, the parent this re-review targets), and stitched by
be-7en1.5 (eb180387f, deferred per be-xeh1 gate). All five ancestors
are PG-area DEFER per ship gate.

## Why DEFER

- §9 fix sits in the same `docs/POSTGRES-BACKEND.md` chain whose
  parent file is not on origin/main.
- The doc describes Postgres-backend code paths (per-prefix
  `issue_counter` schema, `idgen.go` mode dispatch) whose source
  files (`internal/storage/postgres/**`) are absent from
  origin/main.
- Per ship gate: "any file absent from origin/main → DEFER without
  PM escalation."

## Relation to the doc chain

The POSTGRES-BACKEND.md chain ships together:

| Bead | Commit | Gate file |
|---|---|---|
| be-8qo7 (be-5954 skeleton + §2/§6) | f9833070b | be-8qo7-gate.md |
| be-76gb (be-9e34 §1/§3/§4) | 25241ae80 | be-76gb-gate.md |
| be-ohoi (be-xush §5/§7/§11) | d312484c3 | be-ohoi-gate.md |
| be-jhho (§8/§9/§10/§12 — parent) | d5dc18b3f | (closed via be-9myv review) |
| **be-9myv (§9 F1 fix on be-jhho)** | **62a2a5d2f** | **this gate** |
| be-xeh1 (be-7en1.5 stitch + TOC) | eb180387f | be-xeh1-gate.md |

With this F1 fix verified PASS, the §9 factual error flagged in
be-9myv's first round is resolved — the be-xeh1 stitch's
informational HOLD MERGE flag is no longer load-bearing. The full
chain ships as one unit when the PG ship gate lifts.

## Disposition

- Bead closed with reason: `deferred per ship gate, see docs/plans/pg-backend-ship-gate.md`.
- Bead notes carry reviewer analysis (gm-7zw4r1). Preserved for rollup ship.
- Cherry-pick order at rollup time: append `62a2a5d2f` after `d5dc18b3f`
  (be-jhho parent), before `eb180387f` (be-xeh1 stitch).

## PM notification

One-line queue entry mailed to beads/pm for the §9 fix; clears the
HOLD MERGE flag on be-xeh1.

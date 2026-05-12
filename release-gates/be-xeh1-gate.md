# Release Gate — be-xeh1 (DEFER per PG-backend ship gate)

**Disposition:** DEFER. No PR opened. No PM escalation.
**Authority:** memory `deployer-pg-area-defer-no-pm-escalation`
(PM-ratified gm-b01bf6 2026-05-12). Ship-gate doc:
`docs/plans/pg-backend-ship-gate.md`.

## Bead

- ID: be-xeh1 — Review: be-7en1.5 docs/POSTGRES-BACKEND.md TOC + stitch + cross-link verification
- Source commit: `eb180387f`
- Source branch: `fix/be-0d4-postgres-init-guards`
- Reviewer mail: gm-hvtlg6 (PASS with HOLD MERGE flag)

## Gate criteria

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Reviewer PASS present | PASS (gm-hvtlg6, in bead notes) |
| 2 | Acceptance criteria met | PASS (verified by reviewer — 19/19 anchors, 12/12 TOC entries, 5/5 external links) |
| 3 | Cherry-pick to fresh branch off origin/main | **FAIL** — modify/delete conflict |
| 4 | Premise present on origin/main | **FAIL** — see below |

## Cherry-pick verification

Cherry-picked `eb180387f` onto a fresh branch off `origin/main`
(`da73b7511`):

```
CONFLICT (modify/delete): docs/POSTGRES-BACKEND.md deleted in HEAD and
modified in eb18038 (docs(postgres): be-7en1.5 TOC + stitch + cross-link
verification).
```

The stitch commit modifies `docs/POSTGRES-BACKEND.md`, but the file
does not exist on origin/main — it is introduced by be-5954
(f9833070b, deferred per be-8qo7 gate) and filled by siblings be-9e34
(25241ae80, deferred per be-76gb gate), be-xush (d312484c3, deferred
per be-ohoi gate), and be-jhho (d5dc18b3f, review be-9myv with open
request-changes). All four ancestors are PG-area DEFER per ship gate.

## Why DEFER

- Stitch sits on top of all four sibling commits and the parent doc
  itself, none of which are on origin/main.
- The doc describes Postgres-backend code paths (DSN handling,
  init flow, backup error surface, `_project_id` reuse,
  pool params) whose source files (`internal/storage/postgres/**`,
  `cmd/bd/init_postgres.go`) are absent from origin/main.
- Per ship gate: "any file absent from origin/main → DEFER without
  PM escalation."

## Secondary blocker (orthogonal)

Reviewer flagged in gm-hvtlg6 that the underlying §9 content (added
in d5dc18b3f / be-jhho) has an open **request-changes** in be-9myv —
a false-uniqueness claim about the numeric ID suffix that the
issue_counter schema contradicts. The stitch itself is independent
of that fix, but the parent §9 content shouldn't ship until be-9myv
is re-reviewed green.

This is informational only. The ship-gate DEFER blocks merge
regardless.

## Disposition

- Bead closed with reason: `deferred per ship gate, see docs/plans/pg-backend-ship-gate.md`.
- Bead notes carry reviewer analysis (gm-hvtlg6). Preserved for rollup ship.
- The full POSTGRES-BACKEND.md chain (be-8qo7 / be-76gb / be-ohoi /
  be-jhho-after-be-9myv-fix / be-xeh1) ships together when the PG
  ship gate lifts.

## PM notification

One-line queue entry mailed to beads/pm covering the full doc chain
(skeleton + §1/§3/§4 + §5/§7/§11 + stitch).

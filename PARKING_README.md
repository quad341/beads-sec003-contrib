# PG driver work — parked 2026-05-17

This branch parks the Postgres-driver work pursued in May 2026, after the
operator's diligence-over-fork call:

> "i think the correct thing is to be more diligent. move back to dolt in server mode
> (no embedded) and actually take the time to document our pain. shove our pg work
> onto a branch somewhere, but we should try to be part of the community and not
> upset it."

## What's preserved here (branch tip 2026-05-17)

- `pkg/serde` — Row, RowKind, RowExporter, RowImporter primitives (be-azk8my)
- Driver interface + CapabilitySet (be-7ae63g / be-vc5gtl, PR #3980)
- Dolt + PG concrete driver implementations (be-g5ppom / be-aa1cqy)
- Schema-stale guard + bd migrate schema subcommand (be-o7fh35 / be-gudo26 / be-l4z4xw, PR #4015 — closed; upstream issue gastownhall/beads#4016)
- --backend=postgres init stub (be-uo71at, PR #4012 — pending rework on assertion mismatch)

Related branches with finer-grained work:
- `feat/be-7ae63g-lp2mzg-driver-capability` — driver interface alone
- `feat/be-uo71at-init-backend-stub` — --backend stub
- `feat/be-jpdo86-pg-bulk-labels` — PG-specific perf
- `fix/be-y0sm9s-fail-fast-pg-backend` — fail-fast safety
- `fix/be-{2clc,szr}-pg-wisp-*` — PG wisp semantics
- `fix/be-{efr0tm,kil3so}-pg-delete-*` — PG delete paths
- `fix/be-tjiy-pg-filter-coverage` — PG filter parity
- `tests/be-{0d4,2bvr4u,8skfsh,jygyks,nl46qf}-pg-*` — PG test coverage

## Why this is parked, not abandoned

The PG hypothesis (faster, less RAM, fewer crash loops than Dolt) is plausible
but unmeasured. Most of our concrete pain in May 2026 traced to bd's pg-migration
code (gm-r79694 investigation) and stale binaries, not Dolt's storage layer.
Upstream (gastownhall/beads, maintainer @coffeegoddd / @steveyegge) is not
pursuing PG (#1445 closed not planned).

We return to Dolt server mode and rigorously document any future Dolt pain in
HQ bead gm-4lxazp. If structured evidence accumulates (severity, repro, no
upstream fix path), we reopen the PG case with data rather than impressions.

## Refs

- HQ epic that's now superseded: gm-mdoi6b
- Strategy revisit OBE bead: gm-uju4ec
- Pain log bead: gm-4lxazp
- Closed upstream PR: gastownhall/beads#4015
- Filed upstream issue: gastownhall/beads#4016
- Closed deep-investigation: gm-r79694
- Plan doc (now obsolete): /home/jaword/projects/gc-management/docs/plans/pg-driver-ship-plan.md

# PG-doctor parity audit — be-4i7ax3 (architect, 2026-05-11)

**Status:** decomposed; 11 builder beads filed by architect; ALL VANISHED
**Source bead:** be-4i7ax3 (vanished mid-session)
**Architect's mail:** gm-j6m7vd (2026-05-11)
**Architect's canonical doc:**
`/home/jaword/projects/gc-management/.gc/worktrees/beads/architect/architect-be-4i7ax3.md`

## Why this doc exists

The architect filed 11 builder beads from the PG-doctor parity audit
and nudged the 3 P1s. All 11 beads have vanished from `bd show`/`bd
list`/`bd search` per the known phenomenon. The architect's full audit
doc survives in their worktree; this file is the PM-side mirror so the
decomposition surface is preserved for re-filing if needed.

**Important:** the builder may already be working on these (one of the
P1s — be-3l376t / PG dependency cycles — has already landed as commit
`5120ce02f` per `be-3l376t-gate.md`). The vanishing is a tracking
issue, not a work-stoppage. Builder routing happens via mail when bd
loses the bead.

## The 11 beads (from architect's mail + audit §8)

### P1 (ship-gate diagnostic gaps)

| Bead ID | Audit anchor | Title |
|---|---|---|
| **be-8n4fpc** | D-4 | PG repo fingerprint (wrong-DB / DSN-drift detection) |
| **be-oq3lj5** | D-5 | PG referential integrity / orphan-row check |
| **be-3l376t** | D-9 | PG dependency cycles (port from sharedStore graph algo) — **ALREADY BUILT** (commit `5120ce02f`, PASS via gm-cx06pl, gated by ship gate per `be-3l376t-gate.md`) |

### P2 (parity with Dolt-side surface)

| Bead ID | Audit anchor | Title |
|---|---|---|
| **be-a8c3aq** | D-2 | PG Issue Count check |
| **be-5rx5fn** | D-3 | PG per-table schema shape verification |
| **be-ew6m1h** | D-6 | PG config-values validation (DSN, sslmode, identity.actor) |
| **be-wt7maf** | D-19 | PG database size / pg_relation_size summary |

### P3 (polish + verification)

| Bead ID | Audit anchor | Title |
|---|---|---|
| **be-32dbhm** | D-7 | PG multi-repo custom-types discovery |
| **be-b9djwq** | D-8 | PG grants/privileges check (informational) |
| **be-37tg64** | D-10 | Short-circuit Untracked Files check to N/A on PG |
| **be-g6ovx9** | D-11..D-18 | Validation+maintenance cluster verify-then-adapt |

## Cross-references (per architect)

- **be-vd64cw** (open, P3, NOT in the 11) — `bd doctor --check=migration` for PG. Related to be-oq3lj5; different diagnostic, same problem domain. Keep filed as-is.
- **be-5apw1a** (open, P3, NOT in the 11) — 5 LOW polish findings. Item 5 subsumes audit D-1 (table-presence list expansion). Related to be-ew6m1h and be-37tg64 (possible bundle).
- **D-1** (table presence) — subsumed by be-5apw1a item 5; no new bead.

## Builder claim order (architect's recommendation)

1. **P1 first**: be-8n4fpc → be-oq3lj5 → be-3l376t (last is already done).
2. **P2 in parallel** after P1s land.
3. **P3 background** polish.

The architect already nudged the 3 P1s. Patrol will surface the rest.

## Routing & ship-gate interaction

All 11 are PG-backend code (`cmd/bd/doctor/postgres*.go`). All commits
that land from this work-set go through the **PG-backend ship gate**
(see `pg-backend-ship-gate.md` in this folder).

**For reviewer:** when reviewing any of these commits, apply the new
PG-area routing — PASS verdict goes to the ship-gate doc, no
`needs-deploy` label, close with the standard DEFER reason. Do not
route to deployer for the per-commit gate.

## What PM needs to do

- **Nothing for the decomposition itself** — the architect did the
  decomposition and routed it. Builder is the next owner.
- **Re-file beads if vanishing blocks builder claim** — only if the
  builder reports back that they can't find the work. Use this doc
  as the source-of-truth for re-filing.
- **Track verify-cluster outcomes** — be-g6ovx9 may spawn per-function
  follow-up beads; PM should be aware those will appear.

## Out of scope (per architect's §6)

- `runInitDiagnostics` PG path — separate audit pass needed.
- `bd doctor --fix` repair surface — separate design pass.
- `bd doctor --check=conventions` — backend-agnostic.

## Source documents

- Architect's audit doc:
  `/home/jaword/projects/gc-management/.gc/worktrees/beads/architect/architect-be-4i7ax3.md`
  (full methodology, parity matrix of ~60 checks, gap inventory).
- Architect's handoff mail: gm-j6m7vd.
- Already-shipped reviewer evidence: gm-cx06pl (be-3l376t PASS), see
  `be-3l376t-gate.md` for ship-gate disposition.

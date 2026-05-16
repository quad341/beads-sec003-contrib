# Plan: be-rqyhbb handoff — `Count*` aggregate methods

**Source design:** be-rqyhbb (designer, 2026-05-07)
**Architecture parent:** be-yinl4d (architect, closed)
**Pairs with:** be-jaavsb plan (`Iter[T]` interface)
**Owner of plan:** beads/pm
**Date:** 2026-05-07

## Goal

Add aggregate `Count*` methods to `storage.Storage` so callers
needing only cardinality (`len(issues) > 0`, hub-bead progress, epic
closure check) skip row materialization entirely. Storage layer only
— CLI migrations live in be-pkg36q.

## Decomposition

Designer recommended a single PR ("Recommend single PR for the
storage layer"). pm split off a precursor PR (extract
`buildSearchClause`) shared with be-jaavsb so both can land in
parallel — designer's recommended option (b) from be-rqyhbb §11 Q1.

One implementation bead, plus the shared precursor:

| Child | Title | Blocked-by |
|-------|-------|------------|
| be-74hkw2 (P2) | Extract buildSearchClause helper in PG storage (precursor — shared with be-jaavsb plan) | — |
| be-fv0xme (P2) | Implement Count* aggregate methods on storage.Storage | be-74hkw2 |

### be-fv0xme — Count* implementation

Single PR per designer — storage layer only:

- 5 `Count*` methods on `Storage`: `CountIssues`, `CountDependents`,
  `CountDependencies`, `CountIssueComments`, `CountEvents`.
  All return `int64` (designer §2 — match SQL `COUNT(*)` BIGINT
  semantics).
- 1 `CountDependentsByStatus` on `DependencyQueryStore` (placement
  rationale designer §2 — pure dependency-edge query, sits with
  `IsBlocked` / `GetDependencyCounts`).
- PG impls in `internal/storage/postgres/counts.go` (new file) —
  reuse `buildSearchClause` from be-74hkw2.
- Dolt impls in `internal/storage/dolt/counts.go` (new file) — reuse
  `issueops.BuildIssueFilterClauses`.
- `CountEvents(limit > 0)` uses SQL `LEAST(count(*), $2)` clamp at
  the DB level — one round trip, no N-row materialization on hub
  beads.
- `HookFiringStore` pass-throughs (read-side; no hooks fire).
- Parity tests (8 scenarios; the `count == int64(len(GetXxx))`
  equivalence test is the bug-class killer).
- `BenchmarkCountIssuesVsLen` pins the ≥ 50× speedup architect
  requires; record measured number in bench file's comment.

## Coordination

- **be-74hkw2 must land first.** Shared precursor with be-u0zlsq.
- **be-fv0xme blocks be-pkg36q** (CLI count migrations: `bd info`,
  `bd preflight`, `bd close` epic-closure check, `bd ping`).
- be-fv0xme pairs with be-u0zlsq (`Iter[T]` impl). After be-74hkw2
  lands, both can run in parallel review.

## Open questions resolved by design

- **Q1 — buildSearchClause sequencing:** chose option (b) — small
  precursor PR (be-74hkw2). Both impl beads consume it.
- **Q2 — `CountEvents(limit > 0)`:** use `LEAST(count(*), $2)` SQL
  clamp. Single round trip; both PG and Dolt's gms support `LEAST`.
- **Q3 — multi-status `CountDependentsByStatus`:** keep single
  status. The `bd close` epic-closure caller needs "any open?" →
  `n, _ := ...(id, OPEN); return n > 0`. Multi-status can be a
  follow-up if a real call site needs it.
- **Q4 — bench RAM assertion:** the 50× len-vs-count ratio is the
  ceiling; don't over-engineer the bench.

## Out of scope

- CLI migrations (be-pkg36q)
- Iter* methods (be-u0zlsq)

## Done-when (plan-level)

- be-74hkw2 merged
- be-fv0xme merged (single PR; storage layer only)
- be-pkg36q (CLI migrations) transitions to ready
- Designer + architect notified via mail

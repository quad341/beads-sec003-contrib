# Plan: be-jaavsb handoff — `Iter[T]` interface + `Iter*` methods

**Source design:** be-jaavsb (designer, 2026-05-07)
**Architecture parent:** be-yinl4d (architect, closed)
**Owner of plan:** beads/pm
**Date:** 2026-05-07

## Goal

Land the canonical streaming-iterator shape for `storage.Storage` so
five queued migration beads (be-c40q2v / be-775o60 / be-7hvi6c /
be-f68axd / be-60kmhm) can compile and be picked up. The interface is
the bottleneck for everything downstream of be-yinl4d.

## Decomposition

Designer recommended a single PR ("interface lands as one unit so
be-c40q2v/be-775o60/be-7hvi6c/be-f68axd/be-60kmhm can compile against
it"). pm split off a small precursor PR (extract `buildSearchClause`)
so be-jaavsb and be-rqyhbb can land in parallel without merge
conflict — designer's recommended option (b) from be-rqyhbb §11 Q1.

Two child beads, both routed to `beads/builder` with label
`ready-to-build`:

| Child | Title | Blocked-by |
|-------|-------|------------|
| be-74hkw2 (P2) | Extract buildSearchClause helper in PG storage (precursor) | — |
| be-u0zlsq (P1) | Implement Iter[T] interface + Iter* methods on storage.Storage | be-74hkw2 |

### be-74hkw2 — buildSearchClause precursor

Pure refactor — no behavioral change. Extract the WHERE-clause/args
build path from `internal/storage/postgres/issues.go::SearchIssues`
into a private helper. Both be-u0zlsq and the be-rqyhbb child
(be-fv0xme) depend on it.

This bead is small. Land it first; both impl beads consume it.

### be-u0zlsq — Iter[T] interface + skeletons

The full interface in one PR per designer:

- `Iter[T any]` generic interface in `internal/storage/iter.go` (new)
- 9 `Iter*` methods on `storage.Storage` (full surface from be-yinl4d §8)
- 1 `IterAllDependencyRecords` on `DependencyQueryStore`
- Fully-streaming PG + Dolt impls for `IterIssues` +
  `IterDependentsWithMetadata` (the two highest-leverage; pin the
  dedicated-pool-conn deadlock guardrail per design §4.1)
- Stub-then-slice impls for the other 8 via `storage.SliceIter[T]`
- `ForEach[T]` + `Collect[T]` helpers in `iter.go`
- `HookFiringStore` pass-throughs for all 10
- Parity tests (8 scenarios incl. concurrent-iterators test that
  pins the dedicated-conn requirement)
- Gob audit on `types.Issue` / `types.Comment` / `types.Event` /
  `types.Dependency` — unblocks be-oyer9z (RPC iterator transport)

## Coordination

- **be-74hkw2 must land first.** It's the shared dep between
  be-u0zlsq (this plan) and be-fv0xme (be-rqyhbb plan).
- **be-u0zlsq blocks 5 follow-up beads** (be-c40q2v / be-775o60 /
  be-7hvi6c / be-f68axd / be-60kmhm — all open, awaiting interface).
  These are the migrations that consume the new shape. They will be
  unblocked the moment be-u0zlsq lands.
- **be-u0zlsq also unblocks be-oyer9z** (bdd daemon RPC iterator
  transport). The gob-audit deliverable is the gate.
- be-u0zlsq pairs with be-fv0xme (Count* impl). After be-74hkw2
  lands, both can run in parallel review.

## Out of scope

- CLI migrations (live in the 5 blocked beads above)
- Per-method streaming impls beyond the two reference impls (deferred
  to follow-up children of be-yinl4d)
- RPC iterator transport (be-oyer9z, downstream)

## Open questions resolved by design

- **Iter shape:** generic `Iter[T any]` (not per-type interfaces).
  Decided by architect, confirmed by designer §2.
- **Pointer vs value:** `Value() *T` to match slice shape
  (`[]*types.Issue`).
- **Memory model:** "may be reused" — keeps zero-alloc path open;
  callers retaining must copy.
- **Per-row hydration in PG:** dedicated pool conn for the cursor;
  hydration runs on a SECOND conn. Concurrent-iterator parity test
  pins the requirement.

## Done-when (plan-level)

- be-74hkw2 merged
- be-u0zlsq merged (single PR)
- 5 dependent migration beads transition to ready
- be-oyer9z (RPC iterator transport) unblocked
- Designer + architect notified via mail

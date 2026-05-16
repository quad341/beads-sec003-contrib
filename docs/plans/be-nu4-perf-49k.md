# Plan: bd performance at 49K-bead scale

**Source ADR:** be-nu4 (architect, 2026-04-23)
**Source designer audit:** notes on be-nu4.1 / be-nu4.2 / be-nu4.3 / be-nu4.4
**Owner of plan:** beads/pm (this doc)
**Date:** 2026-04-24

## Goal

Land four independent storage-layer changes (D1-D4) that bring `bd
count` / `bd list --all` / `bd stale` / `bd query` from multi-second
or 20s+ wall times down to the per-command FRs in be-nu4 §2 — at
49K beads (~16K wisps), measured.

**No CLI contract changes. No new caches/triggers/materialized
views. Storage-interface additions only.**

## Decomposition

Each architect-approved decision (D1-D4) becomes one ready-to-build
bead routed to the builder. D3 also gets a pre-build ratification
bead routed to the architect, and D4 gets a sibling UX bead so the
on-disk migration changes don't ship without a runner-output story.

| Bead         | Role          | Routes to        | Status        | Notes                                       |
|--------------|---------------|------------------|---------------|---------------------------------------------|
| be-nu4.1.1   | D1 build      | beads/builder    | ready-to-build | `CountIssues` / `CountIssuesGroupedBy`     |
| be-nu4.2.1   | D2 build      | beads/builder    | ready-to-build | `WispIDSetInTx` + refactor 2 hot loops     |
| be-nu4.3.1   | D3 addendum   | beads/architect  | needs-architecture | Ratify `Pinned` + scope `--long` boundary |
| be-nu4.3.2   | D3 build      | beads/builder    | blocked        | Blocked on .3.1 (architect) + .2.1 (D2)    |
| be-nu4.4.1   | D4 build      | beads/builder    | ready-to-build | Migration 0033: date indexes on `issues`   |
| be-8ja       | D4 sibling UX | beads/builder    | ready-to-build | Migration runner stderr progress + warning |

## Dependency graph

```
be-nu4 (ADR, closed)
├── be-nu4.1 (design, in_progress)
│   └── be-nu4.1.1 (D1 build) ────────────────────────► builder
├── be-nu4.2 (design, in_progress)
│   └── be-nu4.2.1 (D2 build) ────────────────────────► builder
├── be-nu4.3 (design, in_progress)
│   ├── be-nu4.3.1 (addendum) ───────────────────────► architect
│   └── be-nu4.3.2 (D3 build) ───┐                   ► builder
│                                ├── BLOCKED BY be-nu4.3.1
│                                └── BLOCKED BY be-nu4.2.1
├── be-nu4.4 (design, in_progress)
│   └── be-nu4.4.1 (D4 build) ────────────────────────► builder
│       └── related: be-8ja (sibling UX) ──────────► builder
└── be-nu4.5 (D5 follow-up, P2, in backlog)

```

## Ordering

- **D1, D2, D4** are independent and may land in any order.
- **D3** must wait on:
  1. `be-nu4.3.1` (architect ratifies addendum), AND
  2. `be-nu4.2.1` (D2 wisp set lands first or with D3, per ADR §12).
- **D4 sibling UX** (`be-8ja`) does NOT block D4 development. It is
  a release-gate semantic: do not cut a release containing D4 until
  the sibling lands too.

Builder may pick up D1, D2, D4, and the D4 sibling concurrently.

## Design escalations carried into build beads

### D3: `IssueSummary` projection — Pinned + Metadata

Designer audit (notes on be-nu4.3 §1) found the ADR-defined
projection silently regresses two existing list-renderer features:

- `Pinned` (used by `bd pin`, `cmd/bd/list_format.go:24-29`) — drops
  the `📌 ` prefix from rendered output.
- `Metadata` (used by `--long`, `cmd/bd/list_format.go:107-113`) —
  drops the `Metadata: N keys` line.

**Pm resolution:** add `Pinned` to the projection; scope
`SearchIssueSummaries` migration to compact + agent only; leave
`--long` on `SearchIssues`. This is filed as `be-nu4.3.1` for
architect ratification because amending the ADR's "intentionally
excludes" enumeration is an architectural call, not a pm-time scope
clarification — even though the resolution is small.

The render-parity test in the D3 build bead is a hard gate:
byte-for-byte equality of compact + agent renders, with both a
pinned permanent issue AND a pinned wisp in the fixture.

### D4: silent migration runner UX

Designer audit (notes on be-nu4.4 §2) found that `MigrateUp` has no
user-visible output. D4's index creation will block the terminal for
~25-75s on a 49K-row rig. ^C reflex risks half-applied migrations.

**Pm resolution:** filed sibling bead `be-8ja` for stderr progress
output + a large-rig warning. Plain text only (no spinners — screen
reader friendly, pipeline safe). Independent PR; release-gate
semantic, not a development blocker.

## Acceptance gates (taken from ADR + designer audits)

| FR    | Command                                | Budget at 49K | Bead         |
|-------|----------------------------------------|---------------|--------------|
| FR-1  | `bd count`                             | ≤ 250 ms p95  | be-nu4.1.1   |
| FR-2  | `bd count --by-<field>` (non-label)    | ≤ 500 ms p95  | be-nu4.1.1   |
| FR-3  | `bd count --by-label`                  | ≤ 1s p95      | be-nu4.1.1   |
| FR-4  | `bd list --all` (compact)              | ≤ 2s p95      | be-nu4.3.2   |
| FR-5  | `bd stale` / date-filter `bd query`    | ≤ 500 ms p95  | be-nu4.4.1   |

Plus hard gates:

- D1 filter parity: `CountIssues(filter) == len(SearchIssues("", filter))`
  for every filter dimension at 1K.
- D2 mixed-ID routing test: routes perms and wisps to the right
  tables in both refactored helpers.
- D3 render parity: byte-for-byte equality of compact + agent
  renders, with pinned-perm + pinned-wisp fixtures.
- D4 write-regression ceiling: hard 10% on `BenchmarkCreateIssue`
  and `BenchmarkUpdateIssue` at 10K rows. Re-escalate to architect
  if exceeded; do NOT land.

## Out of scope (in this batch)

- D5 denormalized counts for `bd stats` — deferred to be-nu4.5
  (P2 follow-up, gated on a new ADR).
- Replacing Dolt, multi-node topology, read replicas — out of scope
  per ADR §10.
- Indexing `wisps` date columns — out of scope per ADR §4.D4.
- Composite indexes — re-escalation path if D4 violates the write
  gate, NOT a builder pivot.

## Sequencing for builder

Pickup order (highest leverage first, mostly independent):

1. **D2 build (`be-nu4.2.1`)** — unlocks D3 and improves all
   bulk-hydration paths.
2. **D1 build (`be-nu4.1.1`)** — single biggest user-facing latency
   win (`bd count` 20s+ → sub-second).
3. **D4 build (`be-nu4.4.1`)** — schema-only, independent.
4. **D4 sibling UX (`be-8ja`)** — release-gate alongside D4.
5. **D3 build (`be-nu4.3.2`)** — picks up after D2 lands and
   architect ratifies `be-nu4.3.1`.

## Open risks

- **Dolt push divergence** — `bd dep add` warns about diverged
  remote histories. Local writes succeed (config has
  `no-push = true`), but if other agents in this rig don't see the
  new beads via shared storage, the dep edges (especially the
  blocking edges on D3) won't gate `bd ready`. Pm to verify after
  handoff that builder's `bd ready --metadata-field
  gc.routed_to=beads/builder` returns the new D1/D2/D4 beads but
  NOT D3 build.
EOF

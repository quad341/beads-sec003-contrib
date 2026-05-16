# Plan: D4v2 build — composite (status, updated_at) + standalone defer_until

**Source ADR addendum:** be-eei (architect, 2026-04-24; ratified)
**Designer audit:** be-yhs (notes section)
**Owner of plan:** beads/pm
**Date:** 2026-04-25

## Goal

Land migration 0033 with the **composite-minimal** index design:

1. Composite `idx_issues_status_updated_at` on `(status, updated_at)`,
   replacing single-column `idx_issues_status`.
2. Standalone `idx_issues_defer_until` on `defer_until`.

This is the re-scoped follow-up to be-nu4.4.1, which exceeded the
hard 10% `CreateIssue_10K` write-regression ceiling with the
original five-single-column design. The composite-minimal set is
projected at ~+4.4% regression with comfortable noise headroom (see
be-eei §4 write-cost model).

## Decomposition

The architect + designer have done all up-front work. The build
work is one tightly-scoped bead — splitting would introduce
coordination cost on a single PR's gates.

| Bead    | Role         | Routes to        | Status         | Notes                                                                  |
|---------|--------------|------------------|----------------|------------------------------------------------------------------------|
| be-s54  | D4v2 build   | beads/builder    | ready-to-build | Migration 0033 + EXPLAIN gate + bench gates + read-bench gate          |
| be-7zf  | Follow-up    | (deferred, P2)   | deferred       | Indexes for closed_at / started_at / due_at — not promoted in this pass |

## Why one bead and not several

The §8 guardrails name distinct deliverables (DDL, down.sql,
round-trip test, EXPLAIN, bench characterization, read-bench, schema
spot-check), but they all gate the same PR. Splitting would:

- Produce orphan beads waiting on each other for evidence one PR
  must bundle anyway.
- Lose the "all gates pass at once" semantics that be-eei §8
  designed-for.

This is faithful to the architect+designer intent (be-eei §13 and
be-yhs handoff section).

## Dependency graph

```
be-eei (ADR addendum, closed)
└── be-yhs (D4v2 design audit, in_progress → closed by pm)
    └── be-s54 (D4v2 build) ──────────────────────────► builder
        └── related: be-eei (binding source)
        └── discovered-from: be-yhs

be-7zf (P2, deferred to 2026-07-24) — not in this scope
```

## Hard gates carried into the build bead

All from be-eei §8, plus the CHANGELOG line drafted in be-yhs §2:

1. **EXPLAIN evidence (§8.4).** PR must show planner picks
   `idx_issues_status_updated_at` for the `bd stale` predicate AND
   `idx_issues_defer_until` for the deferred-parents predicate. If
   either fails, **re-escalate to architect** — do not edit the
   index set inline.
2. **Write regression ≤ 10% (§8.5–6).** `CreateIssue_10K` and
   `UpdateIssue_10K`, both with `-count≥5`, benchstat `-geomean`,
   and a characterized noise envelope. Address the testcontainer
   EOF condition from be-nu4.4.1 in the harness (persistent Dolt
   process if needed).
3. **Read benchmark hard gate (§8.7).** Ship
   `BenchmarkGetStaleIssues_{1K,10K,50K}` numbers. FR-5 (≤ 500 ms
   p95 at 49K-scale equivalent) must be measured this PR — be-nu4.4.1
   skipped it.
4. **Round-trip test (§8.3).** Up → down → up at 2K rows.
5. **Schema spot-check (§8.8).** Update
   `internal/storage/embeddeddolt/schema_test.go:95` to expect
   `idx_issues_status_updated_at` and `idx_issues_defer_until`,
   removing `idx_issues_status`.
6. **`.down.sql` exact reverse (§8.2).** Verify by inspection.
7. **No silent scope creep (§8.9).** No additional indexes in this
   PR even if benchmarks show headroom. The deferred set is be-7zf.

## Migration-runner UX: nothing new asked

Designer audit be-yhs §1 confirmed be-8ja (shipped 2026-04-24) is
sufficient for the revised migration shape. The "up to 60 seconds"
warning is now conservative (real wall time ~10–25s on 49K) but is
deliberately left alone — over-warning beats under-warning when
defeating the ^C reflex. Negative-value churn to edit.

## CHANGELOG line (designer-drafted, builder-trimmable)

> * **perf(schema):** composite `(status, updated_at)` and standalone
>   `defer_until` indexes on `issues` (migration 0033). Speeds up
>   `bd stale` and `bd ready` on large rigs. First run after upgrade
>   applies the migration — expect a one-time 10-25s pause on rigs
>   with >10K issues, with progress shown on stderr. Do not
>   interrupt.

Order matters: win first, pause second, don't-interrupt imperative
last.

## Reusable predecessor artifacts (builder worktree)

In `/home/jaword/projects/gc-management/.gc/worktrees/beads/builder/`:

- `internal/storage/schema/migrations/0033_add_date_indexes.up.sql.disabled`
  → rewrite as 3-statement DDL (DROP idx_issues_status, CREATE
  composite, CREATE defer_until), rename to `.up.sql`.
- `internal/storage/schema/migrations/0033_add_date_indexes.down.sql`
  → rewrite to exact reverse.
- `internal/storage/dolt/migration_0033_test.go` → keep shape;
  update index names.
- `internal/storage/dolt/dolt_benchmark_test.go` → bench additions
  reusable as-is.

## What's out of scope

- Indexes on `closed_at`, `started_at`, `due_at` (be-7zf, deferred).
- Migration runner UX (be-8ja, closed).
- `wisps` date columns (parent ADR be-nu4 guardrail).
- Re-deciding the index set. If benchmarks fail, re-escalate to
  architect — do not redesign in-line.

## On re-escalation

If hard gate 1, 2, or 3 fails, the builder files a
`needs-architecture` bead routed to `beads/architect` with the
failure evidence. The decision-trail discipline is load-bearing
(be-eei §8.9, parent ADR).

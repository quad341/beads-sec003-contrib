# PG-backend verb matrix audit

> **Parent bead:** be-21updd
> **Snapshot date:** 2026-05-11
> **Source:** `internal/storage/postgres/stubs.go` + `cmd/bd/*.go` cross-reference against `feat/be-u0zlsq-iter-interface` HEAD in the builder worktree.

## Method 1: Storage-interface implementation status

The PG store satisfies the `storage.Storage` interface plus the auxiliary interfaces (`Transaction`, `BulkIssueStore`, `CompactionStore`, `MergeSlotStore`, etc.). The `stubs.go` file lists every method that exists only to satisfy the compile-time interface assertion and currently returns the typed sentinel `errNotImplemented` (with the canonical string `"postgres: <method>: postgres: method not yet implemented in v1 (see bead be-6fk.3)"`).

The 15 stubbed methods, grouped by interface:

### BulkIssueStore (4 stubs)
| Method | CLI verbs that fail | Severity for ship-gate |
|---|---|---|
| `DeleteIssues(ctx, ids, cascade, force, dryRun)` | `bd delete`, `bd prune`, `bd purge` | **HIGH** — operator-facing cleanup |
| `DeleteIssuesBySourceRepo(ctx, sourceRepo)` | `bd repo` cleanup paths | MEDIUM — rare administrative use |
| `UpdateIssueID(ctx, oldID, newID, issue, actor)` | `bd rename` | LOW — rare |
| `PromoteFromEphemeral(ctx, id, actor)` | `bd promote` | LOW — comment-promote flow |

### MergeSlot (Storage) (4 stubs)
| Method | CLI verb that fails | Severity |
|---|---|---|
| `MergeSlotCreate(ctx, actor)` | `bd merge-slot create` | LOW — v2 per stubs.go comment |
| `MergeSlotCheck(ctx)` | `bd merge-slot check` | LOW |
| `MergeSlotAcquire(ctx, holder, actor, wait)` | `bd merge-slot acquire` | LOW |
| `MergeSlotRelease(ctx, holder, actor)` | `bd merge-slot release` | LOW |

### Metadata slots (Storage) (3 stubs)
| Method | CLI verb that fails | Severity |
|---|---|---|
| `SlotSet(ctx, issueID, key, value, actor)` | `bd kv slot set` (issue-scoped slot K/V) | LOW |
| `SlotGet(ctx, issueID, key)` | `bd kv slot get` | LOW |
| `SlotClear(ctx, issueID, key, actor)` | `bd kv slot clear` | LOW |

### CompactionStore (4 stubs — entire surface)
| Method | CLI verb that fails | Severity |
|---|---|---|
| `CheckEligibility(ctx, issueID, tier)` | `bd compact <id> --tier N` | LOW — v2 |
| `ApplyCompaction(ctx, issueID, tier, ...)` | `bd compact <id>` write path | LOW — v2 |
| `GetTier1Candidates(ctx)` | `bd compact --tier1` discovery | LOW |
| `GetTier2Candidates(ctx)` | `bd compact --tier2` discovery | LOW |

## Method 2: Public CLI-verb matrix

These are the top-level verbs that touch the storage layer. PG path is what an operator running `bd init --backend=postgres && bd <verb>` would see.

| Verb | Storage method(s) | PG status | Evidence |
|---|---|---|---|
| `bd init --backend=postgres` | `Open`, schema migrations | **WORKS** | `init_postgres.go`, schema migration 0001/0002/0003 land via `pgx` |
| `bd create` | issue insert, dep insert, label insert, audit event | **WORKS** | `issues.go`, integration tests green |
| `bd list` | `SearchIssues` | **WORKS** | `integration_test.go` 21+ filter subtests green |
| `bd ready` | `SearchIssues` filter | **WORKS** | shares SearchIssues path |
| `bd show` | `GetIssue` + dep / label / comment fan-out | **WORKS** | `issues.go` |
| `bd update` | `UpdateIssue` (per-field) | **WORKS** | `be-3lef9d` PASS, 4/4 integration green |
| `bd close` / `bd reopen` | `UpdateIssue` status field | **WORKS** | shares UpdateIssue path |
| `bd dep add` / `dep remove` / `dep tree` | dependency table CRUD | **WORKS** | `dependencies.go` |
| `bd label add` / `tag` | label table CRUD | **WORKS** | `labels.go` |
| `bd comment` / `bd comments` | comment table CRUD | **WORKS** | `comments.go` |
| `bd search` | `SearchIssues` advanced filters | **WORKS** | `be-jdeief` 20-filter coverage PASS |
| `bd export -o issues.jsonl` | `SearchIssues` + iter | **WORKS** | `iter_issues.go` |
| `bd import` | `BulkUpsertIssues` (via batch) | **WORKS** | round-trip integration test green |
| `bd migrate --to=postgres` | `MigrateFromDB` (one-tx) | **WORKS, narrow tests** | `internal/storage/migration/roundtrip_test.go`; broader tests in be-6ryglx + be-njwzu1 |
| `bd doctor` | backend health checks | **PARTIAL** | PG checks exist (`cmd/bd/doctor/postgres.go` on builder branch); parity-vs-Dolt audit pending |
| `bd doctor --check=migration` | migration-state inspection | **MISSING** | be-vd64cw open |
| `bd delete <id>` | `DeleteIssues` | **BROKEN — NotImplemented** | stub returns errNotImplemented |
| `bd prune` | `DeleteIssues` (via purgeScope) | **BROKEN — NotImplemented** | purge.go:236 |
| `bd purge` | `DeleteIssues` (via purgeScope) | **BROKEN — NotImplemented** | purge.go:184 |
| `bd rename` | `UpdateIssueID` | **BROKEN — NotImplemented** | stub |
| `bd promote` | `PromoteFromEphemeral` | **BROKEN — NotImplemented** | stub |
| `bd repo <subcommand>` cleanup | `DeleteIssuesBySourceRepo` | **BROKEN — NotImplemented** | repo.go calls stub |
| `bd merge-slot <subcommand>` | `MergeSlot*` (4 methods) | **BROKEN — NotImplemented** | stubs |
| `bd kv slot ...` (issue-scoped slot) | `SlotSet`/`SlotGet`/`SlotClear` | **BROKEN — NotImplemented** | stubs |
| `bd compact <id>` / `--tier1/2` | `CompactionStore` (4 methods) | **BROKEN — NotImplemented** | entire surface stubbed |
| `bd backup` | dolt-only command | **NOT APPLICABLE** | `cmd/bd/backup_dolt.go` only; pg uses `pg_dump` per designer's POSTGRES-BACKEND.md §7 |
| `bd dolt ...` | dolt-only subcommands | **NOT APPLICABLE** | by design |
| `bd flatten` / `bd gc` | dolt-only history maintenance | **NOT APPLICABLE** | by design |
| `bd compact` (without --tier or id) | dolt-history compaction | **NOT APPLICABLE** | dolt path; PG compaction is a different surface (see CompactionStore above) |
| `bd federation` / `bd ship` | sync workflow | UNCONFIRMED | needs separate audit pass |
| `bd batch` | multi-write transaction | **WORKS** | `transaction.go` |
| `bd config` (subcommands) | KV config table + SetConfig dispatch | **WORKS** | `be-yl8wc4.3` PASS, `set_config_dispatch_test.go` 4/4 green |
| `bd swarm` / `bd mol` / `bd cook` | epic/molecule path | UNCONFIRMED | wisp tables exist; needs verb-level smoke pass |

## Method 3: Test coverage snapshot

**PG-package tests** (`internal/storage/postgres/`):

| File | Build tag | Function count | Notes |
|---|---|---|---|
| `credentials_test.go` | (none — pure unit) | 3 | DSN redact + sentinel password leak guards |
| `issues_test.go` | (none — pure unit) | 1 | `TestPgGlobToLikePattern` only |
| `integration_test.go` | `integration_pg` | 25 | testcontainers; full lifecycle + 21 search filter sub-cases |
| `benchmark_test.go` | `integration_pg` | 3 | `BenchmarkBdReady`, `BenchmarkBdList`, `BenchmarkBdCreate` |

Plus on the feature branch HEAD (builder worktree):
- `custom_sync_test.go` — 7 subtests for syncCustomTypesPg / syncCustomStatusesPg
- `set_config_dispatch_test.go` — 4 subtests for be-yl8wc4.3 dispatch
- `migration_0003_test.go` — backfill test for be-yl8wc4.4
- `round_trip_test.go` — end-to-end pg-sync (be-yl8wc4.5)
- `auth_hint_test.go` — auth error hint

**Migration tests** (`internal/storage/migration/`):
- `roundtrip_test.go` — Dolt → PG → Dolt single-direction round-trip
- be-6ryglx open (one-tx assertion)
- be-njwzu1 open (10k-issue soak corpus + interrupted-resume)

**Doctor tests** (builder worktree):
- `postgres_test.go` + `postgres_integration_test.go` — doctor-side PG checks
- LOW findings catalogued in be-5apw1a

**Coverage gaps surfaced by this audit:**
1. No soak harness exercising memory/heap under sustained load (mayor's ship-gate criterion #1).
2. No reliability/failure-mode test harness (ship-gate criterion #2).
3. No operational-predictability metric (ship-gate criterion #3).
4. No Dolt-vs-PG latency benchmark comparator (mayor wants latency reported even though not binding).
5. No parity audit of `bd doctor` Dolt-side vs PG-side checks beyond ad-hoc be-5apw1a + be-vd64cw.

## Method 4: Vanished-review-bead followup (audit item 3)

The three review beads flagged as vanished in be-nwwe75 — **be-g352o7, be-8y58uy, be-65a0l2** — are **all findable and CLOSED with PASS verdicts in-bead** as of this audit. No re-review is needed:

| Review bead | Commit | Status | PASS verdict location |
|---|---|---|---|
| be-g352o7 | 9aa59b38f (be-yl8wc4.1) | CLOSED | NOTES section, in-bead |
| be-8y58uy | b1cf8f6de (be-yl8wc4.2) | CLOSED | NOTES section, in-bead |
| be-65a0l2 | 026550e76 (be-yl8wc4.3) | CLOSED | NOTES section, in-bead + deployer gate-doc |

The "vanishing phenomenon" memory from 2026-05-11 documents the cause (embedded-vs-shared dolt sync state in gc-rig sessions). The beads themselves have re-emerged with full verdict text intact, so the LOCAL-PASS calls in be-nwwe75 are backed by visible evidence. **No work needed for this audit item.**

## Method 5: `be-6fk.3` reference cleanup (audit item 2)

The string `be-6fk.3` appears in three places in the PG backend:

| File:line | Context | Action |
|---|---|---|
| `internal/storage/postgres/errors.go:38` | doc comment for `errNotImplemented` | Replace pointer with this audit's parent bead (be-21updd) and/or the new lifecycle-parity epic ID once filed. |
| `internal/storage/postgres/errors.go:41` | sentinel error string seen by users | Same — update the user-facing error to point to a findable bead. |
| `internal/storage/postgres/idgen.go:72` + `transaction.go:143` + `integration_test.go:34` | comments using `be-6fk.3` as an *example* hierarchical ID, not a reference to the bead | LEAVE — these are illustrative, not dangling. |

A small build bead (filed below) replaces the two `errors.go` references; the others are valid illustrative uses of the hierarchical-ID format and stay.

## Method 6: Critical path to "PR-ready" (lift be-nwwe75)

Mayor's lift criteria (from be-nwwe75 + 2026-05-11 mayor clarification):
1. **Memory** — RSS/heap under load wins or ties Dolt
2. **Reliability** — recovery time / failure-mode behavior
3. **Operational predictability** under sustained load
4. **Latency** — reported (not binding); PG does not need to beat Dolt

Critical-path beads (to be filed, prefixed `[CP]` in the decomp summary):
- **[CP-1]** Soak harness design (architect) → implementation → run → report
- **[CP-2]** Benchmark comparator design (architect) → implementation → run → report
- **[CP-3]** Lifecycle parity: implement DeleteIssues + DeleteIssuesBySourceRepo so `bd delete` / `bd prune` / `bd purge` / `bd repo` cleanup all work on PG. (Mayor-scope operators will hit these in normal use.)
- **[CP-4]** Doctor parity audit (architect) so post-ship operators can diagnose PG installs with the same fidelity as Dolt installs.
- **[CP-5]** Migration validation completion (be-6ryglx + be-njwzu1 + be-vd64cw — already exist; this audit bumps their priority to feed CP).
- **[CP-6]** `be-6fk.3` reference replacement in errors.go (trivial; can land independently).

Non-critical, post-ship or v2-deferred:
- UpdateIssueID, PromoteFromEphemeral (bd rename / bd promote — file but P2/P3, not blocking)
- MergeSlot stubs (file P3, post-v1)
- Slot* stubs (file P3, post-v1)
- CompactionStore stubs (file P3, post-v1 — entire surface deferred per stubs.go preamble)
- POSTGRES-BACKEND.md gap (already deferred via be-eougy8 chain)
- be-5apw1a polish (already P3)

When CP-1 through CP-5 land (and their data feeds reach mayor in a form suitable for the ship-gate decision), be-nwwe75 unblocks one way or the other:
- Data shows PG wins on memory + reliability → rollup-ship bead with the cherry-pick block from be-nwwe75 lands.
- Data shows no win → be-nwwe75 closes as "design exercise validated abstraction; not shipping" and the local branch persists as research.

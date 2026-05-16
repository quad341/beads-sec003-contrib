# be-yl8wc4 — pg standalone custom_types/custom_statuses parity

> **PM handoff** — beads/pm → beads/builder
> **Date:** 2026-05-11
> **Parent epic:** be-yl8wc4 (P1 bug)
> **Source chain:** architect → designer → pm → builder
> **Plan format:** single-read context for the builder. Per-bead detail
> lives in each child bead's `notes` (designer pass attached the same
> shared review to all five, with a per-child summary table at the end).

## What and why

`bd init --backend=postgres` produces a database whose `custom_types`
and `custom_statuses` tables are empty, so gascity infrastructure type
validation (`convoy`, `agent`, `molecule`, …) fails on a fresh pg
install. The dolt backend gets this right via runtime sync on
`SetConfig` plus migration `016_backfill_custom_tables`. This work
ports both mechanisms to pg without introducing a `dolt` package
dependency on the pg path.

Full architect spec: be-yl8wc4 description (§1–§14, 15 sections,
sequence diagrams, code sketches).
Full designer review: attached to each child bead's notes (one
shared review).

## Child beads — all P1, all `ready-to-build`, routed to beads/builder

| Bead | Title | Blocked by | Builder notes |
|---|---|---|---|
| be-yl8wc4.1 | export `ParseTypesValue` from issueops | — | Pure rename/export. Spec §6.1, §6.5. No UX surface. **Ready now.** |
| be-yl8wc4.2 | implement `syncCustomTypesPg` / `syncCustomStatusesPg` helpers | .1 | Spec §6.1 verbatim. Wrap errors as `insert custom_type %q: %w` — keep the wording so failure surfaces include the key name. |
| be-yl8wc4.3 | wire `PostgresStore.SetConfig` and `pgxTransaction.SetConfig` to dispatch sync | .2 | **Designer override:** §2 of the designer review. Single-switch dispatch — put the synced-key switch in `pgxTransaction.SetConfig` only; `PostgresStore.SetConfig` becomes a router that fast-paths non-synced keys and routes synced keys through `RunInTransaction(ctx, "", fn)`. Do **not** use `pgx.BeginTxFunc`. Do **not** log on each successful sync (silent runtime sync, mirrors dolt). |
| be-yl8wc4.4 | add `Apply` field to `embeddedMigration` + migration 0003 `backfill_custom_tables` | .1 | Spec §6.2 verbatim. Keep the skip-path log wording short — match dolt-016 up to the version number, no "run bd doctor" hint. Doctor surface is be-flo2kl's domain. |
| be-yl8wc4.5 | integration test — `bd init` → `bd config set` → `bd create` round-trip | .3, .4 | Spec §13 acceptance table. **Add a regression assertion** that on `bd config set status.custom=<bad>` the wrapped error chain still surfaces the key name (designer review §3.1 UX guard). |

### Dependency graph

```
        be-yl8wc4
        /   |    \
       .1   |     |
      / \   |     |
    .2   .4 |     |
     \   /  |     |
      .3    |     |
       \   /      |
        .5        |
```

Sequential: .1 → .2 → .3. Parallel: .4 with .2/.3 (both blocked only
by .1). Last: .5 (blocked by .3 and .4).

### Resolved §15 open questions

1. **Tx idiom for `PostgresStore.SetConfig`** — use
   `s.RunInTransaction(ctx, "", fn)`. Reason: 14 callsites already use
   this idiom across the pg backend (`claim.go`, `comments.go`,
   `issues.go`, `labels.go`, `dependencies.go`). It sets
   `pgx.ReadCommitted`, installs panic-safe rollback, threads
   `IsClosed()`. Introducing `pgx.BeginTxFunc` here would be the only
   divergent style in the package.
2. **Debug log on every runtime sync write?** — no, silent. Mirrors
   dolt (`DoltStore.SetConfig` is silent on every successful write;
   only `016_backfill_custom_tables.go` line 90 logs on parse-error
   skip). Migration 0003 keeps the one skip-path log.

### Out of scope for this batch

- **be-yl8wc4.6** (P3 docs to EXTENDING.md) — explicitly held back by
  the designer. Not routed to builder yet. PM will pick it up from
  the parent's dependents list once the implementation lands.
- **`bd doctor` surface for pg-sync** — owned by be-flo2kl (already
  routed to builder per architect spec §9). Coordinates via the
  documented contract; this batch does **not** prescribe doctor
  checks.

## CLI / UX surface (builder smoke list)

Designer review §3 — three places the user sees this work:

1. `bd config set types.custom='…'` / `status.custom='…'` — success
   is silent (parity with non-synced keys and with dolt). Failure
   wraps as `set config: insert custom_type "foo": <pg error>` or
   `set config: parse status.custom: <detail>`. Integration test in
   .5 should assert the key name survives in the error chain.
2. Migration 0003 first connect — happy path silent. Corrupt-config
   path: one stderr line, `migration 0003: skipping invalid
   status.custom entries: <error>`. No doctor hint.
3. `bd init --backend=postgres` — no new output from this batch;
   migration runs inside `Open` before any user-visible action.

## Acceptance — done when

- All 5 child beads closed.
- `go build ./...` and `go test ./...` pass.
- `bd init --backend=postgres` on a fresh pg DB leaves
  `custom_types` / `custom_statuses` populated from `config.yaml` and
  `config` table contents.
- `bd config set types.custom=...` on pg writes through the synced
  path and is visible to subsequent `IssueType.IsValid` checks in the
  same process.
- Backfill migration 0003 populates existing pg databases without
  losing user-added rows.
- No `dolt` import lands on the pg path.

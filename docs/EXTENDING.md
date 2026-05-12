# Extending bd

Notes for contributors adding a storage backend or otherwise extending
bd's internals.

## Storage backend conventions

### Normalized-projection contract

Some config keys have a **normalized projection table** that mirrors
the config row's value. Currently:

| Config key      | Normalized table   |
|-----------------|--------------------|
| `types.custom`  | `custom_types`     |
| `status.custom` | `custom_statuses`  |

The contract every backend must honor:

- **The config row is the source of truth.** The normalized table is
  a derived mirror; `bd config get types.custom` reads the config
  row.
- **`SetConfig` must atomically write the config row AND replace the
  normalized-table contents to match.** Both writes happen inside a
  single transaction; partial failure rolls the whole thing back.
- **Runtime sync uses DELETE + re-INSERT, not merge.** A
  `bd config set types.custom='["a","b"]'` over an existing
  `["a","b","c"]` must produce exactly `{a, b}` in the normalized
  table — never `{a, b, c}`. Operators hand-editing the normalized
  table via `psql` should expect their edits to be shadowed by the
  next config write.
- **Migration backfill uses `ON CONFLICT DO NOTHING`.** Rows added
  by a one-shot backfill must not overwrite rows already present
  from a prior partial sync.
- **Parse logic stays in `internal/storage/issueops` and
  `internal/types`.** Backend-side sync helpers call the shared
  parsers (`ParseTypesValue`, `ParseCustomStatusConfig`); they do
  not re-implement them.

Reference implementations:

- Dolt: [`issueops.SyncCustomTypesTable`](../internal/storage/issueops)
  called from
  [`internal/storage/dolt/config.go`](../internal/storage/dolt/config.go).
- Postgres: [`syncCustomTypesPg`](../internal/storage/postgres/custom_sync.go)
  called from
  [`PostgresStore.SetConfig`](../internal/storage/postgres/config.go)
  and the `pgxTransaction.SetConfig` tx variant.

A future backend (sqlite, …) gets its own ~15-line sync helper
following the same contract. Architecture rationale and the full
guardrail list live on bead `be-yl8wc4` §10 (run `bd show be-yl8wc4`).

## Lite SELECT shape — `IssueFilter.Lite`

`store.SearchIssues(ctx, query, filter)` accepts an `IssueFilter` value.
When `filter.Lite == true`, the storage layer issues a narrower SELECT
that omits these heavy TEXT columns:

- `description`
- `design`
- `acceptance_criteria`
- `notes`
- `waiters`
- `payload`

### Contract for callers

Code that calls `store.SearchIssues` with `IssueFilter.Lite == true`:

- **MUST NOT** read `Description`, `Design`, `AcceptanceCriteria`,
  `Notes`, `Payload`, or `Waiters` from any returned `*types.Issue`.
  These fields are zero-valued after a lite scan; they did not come from
  the row. Reading them yields no signal.
- **MAY** read every other field — identity, status, priority,
  timestamps, labels, dependencies, metadata, etc. Lite preserves them.
- **MUST** detect lite-fetched records via `issue.IsLitePartial` if
  branching behavior on hydration depth is required. The field is
  internal-only (`json:"-"`) — it never crosses the wire.

To recover the full body for a specific issue after a lite listing,
call `store.GetIssue(ctx, id)` — `GetIssue` always returns the full row.

### Default behavior

`IssueFilter.Lite` defaults to `false`. Every existing call site that
does not opt in retains today's behavior: heavy columns are fully
hydrated, and `Issue.IsLitePartial` is `false`.

### Where the contract is enforced

- Column lists: `internal/storage/issueops/scan.go`
  (`IssueSelectColumns`, `IssueSelectColumnsLite`, `HeavyDropList`).
- Scan helpers: `ScanIssueFrom` (full) and `ScanIssueLiteFrom` (lite,
  sets `IsLitePartial`).
- SELECT dispatch: `internal/storage/issueops/search.go::searchTableInTx`
  switches the SELECT and the scan helper on `filter.Lite`.
- Schema-parity guard:
  `internal/storage/issueops/scan_test.go::TestIssueSelectColumns_LitePlusHeavyEqualsFull`
  fails CI if a future column is added to `IssueSelectColumns` without
  being classified into `IssueSelectColumnsLite` or `HeavyDropList`.

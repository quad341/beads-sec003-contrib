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
  [`PostgresStore.SetConfig`](../internal/storage/postgres/transaction.go)
  and the `pgxTransaction.SetConfig` tx variant.

A future backend (sqlite, …) gets its own ~15-line sync helper
following the same contract. Architecture rationale and the full
guardrail list live on bead `be-yl8wc4` §10 (run `bd show be-yl8wc4`).

# Postgres Storage Backend

Beads supports Postgres as an alternative to the default Dolt backend.
This page documents the Postgres-specific contracts and operator-facing
behavior. For Dolt, see [docs/DOLT-BACKEND.md](DOLT-BACKEND.md).

> **Status:** v1. The `bd migrate --to=postgres` cross-backend data
> copy is supported; backwards migration (`--to=dolt`) is not.

## Reliability bar

This section is the authoritative contract for `bd migrate
--to=postgres`. It is mirrored in the Godoc on
`cmd/bd/migrate.go::handleCrossBackendMigrate`; both copies update
together when the migration implementation changes.

> **`bd migrate --to=postgres` reliability contract**
>
> A successful, non-`--dry-run` invocation guarantees the following
> against the destination DSN, evaluated immediately after the command
> exits `0`:
>
> 1. **Row-count parity, lossless tables.** For each of the eight
>    lossless tables — `issues`, `wisps`, `dependencies`,
>    `wisp_dependencies`, `labels`, `wisp_labels`, `comments`,
>    `wisp_comments` — the destination row count equals the source row
>    count.
> 2. **Carryover present.** The eight configuration tables —
>    `child_counters`, `compaction_snapshots`, `config`,
>    `custom_statuses`, `custom_types`, `issue_counter`,
>    `issue_snapshots`, `metadata` — are populated from the source.
>    `config` and `metadata` are upserted (`ON CONFLICT (key) DO UPDATE
>    SET value = EXCLUDED.value`) over the seed rows shipped by
>    migration 0001; `issue_counter` is upserted on `prefix`. The
>    remaining six are CopyFrom'd over a freshly-empty table.
> 3. **Dependency-graph integrity.** Every `(issue_id,
>    depends_on_id, type)` edge on the source appears with the same
>    `created_at` (UTC) and `metadata` on the destination; no edges are
>    fabricated. Same property for `wisp_dependencies`.
> 4. **Field-by-field equality on bd's typed columns.** Every column
>    enumerated in `internal/storage/migration/copy.go:67-77`
>    (`pgIssueColumns`) is copied through the canonical
>    `issueops.ScanIssueFrom` scanner and projected via
>    `issueToPGRow`. Timestamps are normalized to UTC. JSONB
>    `metadata` is field-by-field equal to the source bytes.
> 5. **Large-blob fidelity.** `description`, `notes`, `design`,
>    `acceptance_criteria`, `compaction_snapshots.snapshot_json`, and
>    `issue_snapshots.original_content` are byte-identical to the
>    source.
> 6. **Audit trail surfaced, not migrated.** `events` and `wisp_events`
>    rows are *not* copied. Their combined count is reported once on
>    stderr (`note: <N> audit-trail events not migrated; see
>    docs/AUDIT_TRAIL_POSTGRES.md`). `--include-events` returns
>    `feature_not_implemented`; it is a v1 placeholder. **An operator
>    relying on audit history must keep the Dolt source.**
> 7. **Source-DB read-only invariant.** The source bd database is only
>    read. The migration never writes to it, never commits to its
>    history, and never holds a write lock on it. Killing the migration
>    process mid-flight cannot leave the source in a damaged state.
> 8. **Atomic destination.** The destination changes are applied in a
>    single Postgres transaction (`ReadCommitted`, one
>    `BeginTx`/`Commit`). Any abort before commit — process killed,
>    network drop, source-read error, destination constraint failure —
>    leaves the destination unchanged. There is **no half-written
>    state** on PG.
> 9. **Idempotency, principled.** Re-running `bd migrate
>    --to=postgres` against a destination that already holds bd data
>    refuses with `destination_not_empty` and a per-table row count.
>    Re-running with `--force` produces a destination indistinguishable
>    from a single fresh migration: TRUNCATE … CASCADE over every
>    migrated table happens inside the same transaction as the copy,
>    so the destination either ends in pre-existing or
>    post-migration state — never a mix.
>
> **Documented, intentional divergences from strict byte-identity:**
>
> - Optional text columns that are empty on Dolt land as SQL `NULL` on
>   Postgres. This is uniform across every nullable string column and
>   reflects PG's idiomatic encoding.
> - `metadata` bytes that are empty on Dolt land as `"{}"` on Postgres
>   (PG `jsonb` does not accept empty input).
>
> These two normalizations are stable: the same input always produces
> the same output. They are *not* parity failures.
>
> **The contract does NOT guarantee:**
>
> - Operational/local tables (`local_metadata`, `repo_mtimes`,
>   `bd_schema_migrations`, `federation_peers`) on the source.
> - Dolt history. Dolt-level diffs, tags, branches are not represented
>   in Postgres.
> - Concurrent migrations to the same DSN (undefined — do not run two
>   at once).

## Recovery from interrupted migration

A killed `bd migrate` process, a dropped destination connection, or a
second run hitting `destination_not_empty` — see
[docs/MIGRATION-RECOVERY.md](MIGRATION-RECOVERY.md) for the recovery
playbook. The atomicity invariant (§2 invariant 8) means the
destination is always in either its pre-migration or post-migration
state; the recovery doc walks through detecting which.

## Audit trail

`bd migrate --to=postgres` does **not** copy the source's `events` and
`wisp_events` tables. An operator who relies on Dolt's audit history
must keep the Dolt source workspace; see
[docs/AUDIT_TRAIL_POSTGRES.md](AUDIT_TRAIL_POSTGRES.md) for the v1
strategy and the post-v1 `--include-events` plan.

## Related

- [docs/MIGRATION-RECOVERY.md](MIGRATION-RECOVERY.md) — recovery from
  interrupted or partial-looking migrations
- [docs/AUDIT_TRAIL_POSTGRES.md](AUDIT_TRAIL_POSTGRES.md) — audit-trail
  strategy on the Postgres backend
- [docs/DOLT-BACKEND.md](DOLT-BACKEND.md) — Dolt backend reference

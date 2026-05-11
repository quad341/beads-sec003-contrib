# Migration Recovery Playbook

This document is the recovery reference for `bd migrate
--to=postgres`. Each named refusal from `bd migrate` and each "what
state am I in?" question points to a labeled anchor below with
step-by-step instructions.

See also: [`docs/POSTGRES-BACKEND.md`](POSTGRES-BACKEND.md) (the
migration reliability contract) and
[`docs/AUDIT_TRAIL_POSTGRES.md`](AUDIT_TRAIL_POSTGRES.md) (audit-trail
strategy).

## Table of contents

- [The mental model in one sentence](#the-mental-model-in-one-sentence)
- [Detecting which state you're in](#detecting-which-state-youre-in)
- [Clean retry (destination is empty)](#clean-retry-destination-empty)
- [Re-migrate over an existing destination](#destination-not-empty)
- [Smoke-test and accept](#smoke-test-and-accept)
- [What to NEVER do](#what-to-never-do)

---

## The mental model in one sentence

The destination is either at its **pre-migration state** (typically the
fresh-init seed: empty lossless tables, seeded config) or its
**post-migration state** (parity with source per the
[reliability bar](POSTGRES-BACKEND.md#reliability-bar)). It is never
between.

---

## Detecting which state you're in

### Step 1 — Did `bd migrate` print a success line?

If `bd migrate --to=postgres --dsn=… --json` returned status
`success` (`internal/storage/migration/migrate.go:170-176` →
`emitCrossBackendMigrateSuccess`), the destination is post-migration.
You are done.

If it returned an error (any JSON field `error: …`) or did not return
at all (process killed, terminal closed, shell hung up), proceed.

### Step 2 — Inspect the destination row counts

Connect to the destination DSN and run:

```sql
SELECT
  (SELECT COUNT(*) FROM issues)             AS issues,
  (SELECT COUNT(*) FROM wisps)              AS wisps,
  (SELECT COUNT(*) FROM dependencies)       AS dependencies,
  (SELECT COUNT(*) FROM wisp_dependencies)  AS wisp_dependencies,
  (SELECT COUNT(*) FROM labels)             AS labels,
  (SELECT COUNT(*) FROM wisp_labels)        AS wisp_labels,
  (SELECT COUNT(*) FROM comments)           AS comments,
  (SELECT COUNT(*) FROM wisp_comments)      AS wisp_comments;
```

Two outcomes:

- **All eight columns are zero.** The transaction never committed.
  The destination is at fresh-init state. Proceed to
  [Clean retry (destination is empty)](#clean-retry-destination-empty).
- **Any column is non-zero.** The transaction committed. Compare each
  count to the source by running the equivalent on the Dolt source
  (`bd doctor` or, on the source workspace, `bd stats`). Two
  sub-cases:
  - **Counts match source.** The migration completed; only the success
    print was lost. Proceed to
    [Smoke-test and accept](#smoke-test-and-accept).
  - **Counts differ from source.** This is unexpected given the
    atomicity invariant; treat as a bug. Capture the destination
    counts and the migration command's stderr/JSON and file a bead
    referencing this document and be-quowfp before doing anything else.

### Step 3 — Marker artifacts to look for

The current migration leaves **no separate marker** beyond the
destination row counts and (if `bd init --backend=postgres` ran first)
the `metadata` / `config` seed rows from migration 0001. There is no
checkpoint file, no `migration_state` table, no `.beads/migrating`
sentinel. The destination row counts ARE the marker.

This is on purpose for v1: the atomic-commit invariant
(reliability bar §8) makes a separate state machine redundant. See
be-quowfp §5 for the follow-up if the assumption weakens.

### Step 4 — `bd doctor`

As of this writing `bd doctor` does **not** inspect a Postgres
destination for migration health. Running it from the source workspace
will report on the source Dolt; running it from a `bd init
--backend=postgres` workspace will report on the destination's general
config but does not currently flag "this destination looks
partially-migrated." A follow-up bead (be-vd64cw) extends doctor with
post-migration sanity checks; until then, use the manual row-count
inspection in Step 2.

---

<a id="clean-retry-destination-empty"></a>

## Clean retry (destination is empty)

The destination is at fresh-init state. Re-run the original command:

```bash
bd migrate --to=postgres --dsn="$DEST_DSN"
```

No cleanup is needed. The empty-destination guard
(`internal/storage/migration/migrate.go:133-141`) will see zero rows
in the lossless tables and proceed.

If the previous run had passed `--source=`, repeat it.

---

<a id="destination-not-empty"></a>

## Re-migrate over an existing destination

**JSON error code:** `destination_not_empty`

The destination already contains bd data — either from a completed
prior migration whose success line was lost, or from an unrelated bd
instance. Two paths:

### 1. Keep the existing destination (success was real)

Run a `--dry-run` and compare the source counts to the destination:

```bash
bd migrate --to=postgres --dsn="$DEST_DSN" --dry-run --json
```

The dry-run reads source counts and destination occupancy without
opening a destination transaction
(`internal/storage/migration/migrate.go:103-127`). If counts match,
the destination is good. Update the workspace's `metadata.json` (or
re-run `bd init --backend=postgres` against the same DSN with
`--reinit-local`) to bind the workspace to the destination, and stop.

### 2. Wipe and re-migrate

Use `--force`:

```bash
bd migrate --to=postgres --dsn="$DEST_DSN" --force
```

`--force` TRUNCATEs every bd-owned destination table CASCADE inside
the same transaction as the copy
(`internal/storage/migration/tables.go:62-70`,
`internal/storage/migration/migrate.go:154-160`). Either the whole
TRUNCATE+copy commits and you have a parity destination, or the
transaction rolls back and the destination retains whatever it had
before. **There is no window in which `--force` leaves the
destination empty or half-cleared.**

### 3. Drop the destination and restart from scratch

Heaviest hammer. Works when you no longer trust the destination
database at all:

```bash
# from a psql session pointed at a superuser DSN, NOT the bd DSN
DROP DATABASE "<bd_db>";
CREATE DATABASE "<bd_db>" OWNER "<bd_role>";
# back in shell
bd init --backend=postgres --dsn="$DEST_DSN" --reinit-local
bd migrate --to=postgres --dsn="$DEST_DSN"
```

This is exactly equivalent to path 2 but useful when the operator
wants the destination schema itself reapplied (e.g., suspecting a
botched manual `psql` edit). Note: dropping the database is
destructive — only do this if no other application uses the database.

---

## Smoke-test and accept

After any successful migration or after confirming a previously-lost
success:

```bash
# Bind the active workspace to the PG destination (no-op if already done):
bd init --backend=postgres --dsn="$DEST_DSN" --reinit-local --quiet

# Sanity checks:
bd stats           # totals should match the Dolt source
bd ready           # should list the same ready issues as the source did
bd doctor          # general config sanity (does not migration-specifically validate; see Step 4 above)
```

If `bd stats` matches and `bd ready` returns the expected work list,
the destination is good. The Dolt source remains untouched and can be
archived or kept as a read-only fallback.

---

## What to NEVER do

- **Never** run two `bd migrate --to=postgres` invocations against the
  same DSN concurrently. The empty-destination guard is not a lock;
  two concurrent runs can both observe an empty destination and race
  to commit. Outcome is undefined (one or both may fail with PG
  constraint errors, or you may double-insert if a future change makes
  the two transactions non-conflicting).
- **Never** manually INSERT / UPDATE / DELETE on the destination's
  bd-owned tables while a migration is in flight or during a retry
  window. The atomic-commit guarantee is what makes recovery trivial;
  hand-edits invalidate it.
- **Never** assume `--include-events` migrated audit history. It
  returns `feature_not_implemented` immediately
  (`internal/storage/migration/migrate.go:57-59, 82-84`) — the whole
  migration is a no-op when `--include-events` is set. If you saw
  audit history on PG after a migration, it came from manual import or
  a prior tool, not from `bd migrate`. See
  [docs/AUDIT_TRAIL_POSTGRES.md](AUDIT_TRAIL_POSTGRES.md) for the
  audit-trail strategy.
- **Never** mutate the Dolt source to "fix things up" mid-recovery.
  The source-read-only invariant
  ([reliability bar §7](POSTGRES-BACKEND.md#reliability-bar)) is the
  property that lets you retry safely; editing the source changes
  what a retry produces.
- **Never** mix sources across retries. If the first attempt used
  `--source=/path/A` and the second uses `--source=/path/B` (or the
  active workspace), you will produce a destination from B that
  inherits the empty-destination semantics from A's prior partial
  attempt only by accident. Pick one source and stick with it.
- **Never** use `pg_hba` `trust` rules as a workaround for the
  password-stripping behavior. Use `BEADS_POSTGRES_PASSWORD` instead.

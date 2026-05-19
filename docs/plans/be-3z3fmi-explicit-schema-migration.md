# Plan: Explicit Schema Migration Command (be-3z3fmi)

**Goal:** Make schema migrations explicit — Store Open refuses to auto-migrate; operators/orchestrators run `bd migrate schema` to apply pending migrations.

**Root bead:** be-3z3fmi  
**Date:** 2026-05-16

## Problem

`schema.MigrateUp` runs silently on every Store Open (embedded Dolt, Dolt server, uow provider). Any `bd ready`, `bd list`, etc. can trigger a slow migration that causes orchestrators to SIGKILL what looks like a hung process.

## Proposed Solution

1. **Store Open paths refuse stale schema** — emit `schema is at version N, binary requires M — run: bd migrate schema` and exit non-zero.
2. **New `bd migrate schema` subcommand** — applies pending migrations explicitly with per-migration progress output.
3. **`bd init` keeps auto-migrate** — fresh install path must stay seamless.

## Child Beads

| Bead | Role | Agent | Status |
|------|------|-------|--------|
| be-sa4ls9 | Architect: design review | architect | ✓ closed |
| be-lw5fak | Build: `schema.PendingMigrationCount()` helper | builder | ✓ closed |
| be-o7fh35 | Build: embedded Dolt Store Open schema guard | builder | ✓ closed (commit 78cf5c5b2) |
| be-gudo26 | Build: Dolt server Store Open schema guard | builder | ✓ closed (commit 78cf5c5b2) |
| be-l4z4xw | Build: `bd migrate schema` subcommand | builder | ✓ closed (commit f7e75872d) |
| be-3zy32x | Tests: schema-stale refusal, all backends | validator | **open — slung 2026-05-19** |
| be-vuf19k | Tests: `bd migrate schema` command | validator | **open — slung 2026-05-19** |
| be-ipgt8m | Chore: CLI docs regen | builder | open |
| be-1vz5e8 | (builder duplicate of be-3zy32x) | — | ✓ closed superseded 2026-05-19 |

**Upstream status:** PR #4015 rejected by coffeegoddd (2026-05-17). Architect decision (be-t4n07l): maintain `feat/be-o7fh35-be-gudo26-schema-guards` as fork-local branch. be-3zy32x tests validate against this branch; tests make rebases verifiable.

## Dependency Graph

```
be-3z3fmi (root)
└── be-sa4ls9 (architect: design review)        ← UNBLOCKED NOW
    ├── be-lw5fak (helper: PendingMigrationCount)
    │   ├── be-o7fh35 (embedded Dolt guard)
    │   │   └── be-3zy32x (tests: stale-schema refusal)
    │   └── be-gudo26 (Dolt server guard)
    │       └── be-3zy32x (same)
    └── be-l4z4xw (bd migrate schema cmd)
        ├── be-vuf19k (tests: migrate schema cmd)
        └── be-ipgt8m (CLI docs regen)
```

## Open Architecture Questions (for be-sa4ls9)

1. Which commands are blocked with stale schema? All, or writes-only?
2. Exact error format + exit code + `--json` error shape?
3. Subcommand naming: `bd migrate schema` vs `bd schema migrate` vs enhance `bd migrate`?
4. Confirm `bd init` auto-migrate exception?
5. Does Postgres backend need same treatment? (MigrateUp not called there currently.)

## Key Files

- `internal/storage/schema/schema.go` — `MigrateUp` and migration logic
- `internal/storage/embeddeddolt/store.go:179` — embedded Dolt Open site
- `internal/storage/dolt/store.go:1484` — Dolt server Open site
- `internal/storage/uow/doltserver_provider.go:175` — Dolt UoW provider Open site
- `cmd/bd/migrate.go` — existing migrate command (678 lines, data/metadata only)

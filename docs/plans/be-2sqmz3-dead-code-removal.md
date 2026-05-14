# Plan: Remove internal/storage/dolt/migrations/ + Close PR #3540

**Root bead**: be-2sqmz3  
**Architect design**: be-dv51qq  
**Date**: 2026-05-14  
**Status**: Decomposed → builder assigned

## Summary

Dead-code removal. `RunCompatMigrations` has zero callers in production — the compat runner was silently retired when all 17 SQL-equivalent migrations were added to `internal/storage/schema/migrations/`. Per coffeegoddd's architectural directive, the entire `internal/storage/dolt/migrations/` subpackage is removed.

Designer confirmed no UX concerns (zero user-facing changes).

## Work Tree

```
be-2sqmz3 (closed — PM decomposition)
└── be-tmnb1g  ready-to-build → beads/builder
    "Delete internal/storage/dolt/migrations/ + close PR #3540"
```

## Child Bead: be-tmnb1g

**Acceptance criteria (abbreviated):**

- Delete `internal/storage/dolt/migrations/` (24 files)
- Delete `internal/storage/embeddeddolt/compat_migrations_test.go`
- Update 4 `sourceFiles` entries in `cmd/bd/doctor/agent.go` → `internal/storage/schema/schema.go`
- Update comment in `internal/storage/doltutil/close.go`
- `go build ./...` passes
- `go test ./...` passes (excluding dolt_only integration tests)
- Post closing comment on PR #3540 per architect spec, then close it
- New PR: `Closes #3540`, `go test` output in description, single-layer, no inline comments

## What Is NOT In Scope

- Changes to `internal/storage/dolt/migrations.go` (the thin wrapper file, not the subpackage)
- Changes to `cmd/bd/migrate.go` (already correct)
- Changes to `internal/storage/schema/migrations/`
- New drift-repair capability

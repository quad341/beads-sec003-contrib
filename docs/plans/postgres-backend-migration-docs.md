# Plan: `bd migrate --to=postgres` documentation rollout

**Status:** Ready for builder.
**Source chain:** be-quowfp (architect) → be-c9cndi + be-gltk3j
(designer) → be-qp10z7 + be-7fmvcx (builder).

## Goal

Land the architect's reliability bar (be-quowfp §2) and recovery
playbook (be-quowfp §3) into the codebase as Godoc + user-facing
docs, so an operator can read the contract and the recovery procedure
without spelunking through bead history.

## Deliverables

Two independent builder beads, both `ready-to-build`, both routed to
`beads/builder`:

| Builder bead | Parent design | What ships |
|---|---|---|
| be-qp10z7 | be-c9cndi | Godoc on `cmd/bd/migrate.go::handleCrossBackendMigrate`, new `docs/POSTGRES-BACKEND.md`, README cross-link, `migrate` Cobra Long-text contract line |
| be-7fmvcx | be-gltk3j | New `docs/MIGRATION-RECOVERY.md`, `migrate` Cobra Long-text recovery line, `docs/AUDIT_TRAIL_POSTGRES.md` TL;DR cross-link |

## Order

The two builder beads are **independent and may land in either
order**. They both touch the `Long:` block of `cmd/bd/migrate.go`
additively (the two added lines do not collide). Whichever lands
first creates the `Cross-backend migration:` block; the second adds
its line inside that block. Per designer's handoff note (mail
`gm-3990t4`).

If both build in parallel and merge concurrently, expect a trivial
merge conflict in the `Long:` block — resolve by keeping both lines.

## How the builder works each one

1. `bd show <builder-id>` → read implementation summary + acceptance criteria.
2. `bd show <parent-design-id>` → read the `design` field for verbatim
   file contents, line numbers, and full acceptance checklist.
3. Apply the patches exactly. Verbatim text is **normative** — do not
   reword.
4. Run quality gates (`gofmt`, `golangci-lint run ./...`, markdown lint).
5. `bd close <builder-id> --reason="…"`.

## Guardrails (from architect — be-quowfp §4)

These apply across both builder beads:

1. The eight tables in invariant 1 must match
   `internal/storage/migration/tables.go:43-52`. The eight in
   invariant 2 must match `:31-38`. Verbatim text in the designs has
   been validated against the current code by the designer; future
   table-list changes update the docs alongside.
2. **Do not promise a marker file or `bd_migration_state` table.**
   The recovery doc uses row counts as the marker; that is
   intentional and normative.
3. The Godoc lives on `handleCrossBackendMigrate` only, not on
   `migration.MigrateFromDB` (which has its package-level Godoc
   already).
4. The "Documented, intentional divergences" enumeration belongs on
   the reliability contract (`docs/POSTGRES-BACKEND.md`), not in the
   recovery doc — the latter cross-links it instead.

## Quality gates summary

- `gofmt` clean.
- `golangci-lint run ./...` — no new warnings (baseline ones OK).
- `go doc github.com/steveyegge/beads/cmd/bd | grep -A1 handleCrossBackendMigrate`
  emits the contract preamble.
- All Markdown TOC links and cross-doc links resolve.
- `docs/MIGRATION-RECOVERY.md` `<a id="…"></a>` markers do not render
  as visible text on GitHub.
- SQL block in MIGRATION-RECOVERY.md lists the eight lossless tables
  in the same order as `internal/storage/migration/tables.go:43-52`.

## Out of scope

Per be-quowfp's "out of scope" line: soak corpus, round-trip diff,
interrupted-resume stress. Those are tracked separately (be-vd64cw,
be-6ryglx — `bd doctor` post-migration sanity checks, etc.).

## Related beads

- be-quowfp — architecture (closed)
- be-c9cndi — designer deliverable for reliability bar (parent of be-qp10z7)
- be-gltk3j — designer deliverable for recovery doc (parent of be-7fmvcx)
- be-vd64cw, be-6ryglx — follow-up `bd doctor` extensions (not in this plan)

# Plan: be-pkg36q — CLI count-only caller migrations

**Source:** be-pkg36q (architect, 2026-05-15)
**Architecture parent:** be-yinl4d (architect, closed)
**Design source:** be-rqyhbb (designer, closed) — CountDependentsByStatus decision §2
**PM:** beads/pm
**Date:** 2026-05-15

## Goal

Migrate four count-only CLI callers from `len(SearchIssues(...))` to
`Count*` aggregate methods (added by be-fv0xme). Each is a one-line
edit; the cumulative O(N)→O(1) RAM saving is material for hub-bead
workflows.

## Decomposition

Architect said "single PR, mechanical sweep." No further decomposition
needed. be-pkg36q itself is the builder bead.

| Bead | Title | Blocked-by |
|------|-------|------------|
| be-pkg36q (P3) | Migration: count-only CLI callers to Count* | be-fv0xme |

be-pkg36q is blocked until be-fv0xme (Count* implementation) lands on
main. When be-fv0xme closes, be-pkg36q will appear in `bd ready` for
the builder automatically.

## Call-site migration map

| Callsite | File | Today | After |
|----------|------|-------|-------|
| `bd info --hub-counts` | `cmd/bd/info.go:65,102` | `SearchIssues + len()` | `CountIssues(ctx, query, filter)` |
| `bd preflight` | `cmd/bd/preflight.go` (all `len(issues)` where slice is discarded) | `SearchIssues + len()` | `CountIssues(ctx, query, filter)` |
| `bd close` epic-closure | `cmd/bd/close.go:455` | `GetDependentsWithMetadata + len()` | `CountDependentsByStatus(ctx, id, types.StatusOpen)` |
| `bd ping` | `cmd/bd/ping.go:54` | `SearchIssues` (probe only) | `CountIssues(ctx, "", IssueFilter{Limit: 1})` |

## Key decisions

- **CountDependentsByStatus** for `bd close` (designer decision,
  be-rqyhbb §2). Sits on `DependencyQueryStore`, not `Storage`.
- **Return type int64** — cast inline where `len() == 0` comparisons exist.
- **bd ping** uses `IssueFilter{Limit: 1}` — equivalent to `SELECT 1`
  for connection probing; no data materialized.
- **bd preflight** — audit all `len(issues)` patterns; migrate only sites
  where the slice itself is discarded after counting.

## Done-when (plan-level)

- be-fv0xme merged (unblocks be-pkg36q)
- be-pkg36q merged: all 4 callsites use Count*
- Tests clean: `go test ./cmd/bd/... -run 'TestInfo|TestPreflight|TestClose|TestPing'`
- Build clean: `go build -tags gms_pure_go ./...` + `go build ./...`

## Out of scope

- Iter* migrations (separate beads in the be-yinl4d tree)
- Multi-status CountDependentsByStatus variant
- Bench (RAM win is code-provable; no bench needed)

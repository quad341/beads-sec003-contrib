# Plan — `bd show --json` dependents/comments to Iter (be-775o60)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-775o60 (designer, 2026-05-15)
**Parent architecture:** be-yinl4d (RAM budget — Iter migrations)
**Dependencies:** be-jaavsb (CLOSED), be-u0zlsq (CLOSED) — IterDependentsWithMetadata and IterIssueComments available

## Goal

Replace slice-based dependent/comment loading in `bd show --json` with count-only by default,
with opt-in streaming via `--include-dependents` and `--include-comments` flags. Eliminates the
memory spike on hub-beads with thousands of dependents.

## Decomposition decision: one builder bead

New flags + streaming path in `cmd/bd/show.go` + test update for PR #3768 regression + bench.
Self-contained. One PR.

Builder bead: be-ijck6q.

## Key design choices

**Default is count-only.** The existing behavior (slice materialization) changes to count-only
for the common case. This is a breaking change to `bd show --json` output format: the
`"dependents"` key is removed from default output; `"dependent_count"` is added instead.

**PR #3768 regression test (a19786496) must be updated.** The test asserted the old slice output.
Builder must update it to assert count-only as the new default, OR add a separate test for
`--include-dependents` to cover the streaming path.

**Streaming format:** `[`, per-item `json.NewEncoder(w).Encode(item)` with comma separator, `]`.
Same pattern as be-c40q2v (`bd list`).

**Flags are `--json`-only.** Text mode silently ignores them (or produces a warning).

## Acceptance (builder bead be-ijck6q)

- `bd show --json` without `--include-dependents`: uses `CountDependents`, no `"dependents"` key.
- `bd show --json --include-dependents`: uses `IterDependentsWithMetadata`, streaming array.
- `bd show --json` without `--include-comments`: uses `CountIssueComments`, count only.
- `bd show --json --include-comments`: uses `IterIssueComments`, streaming array.
- PR #3768 regression test adapted to new default.
- Bench: hub-bead 10K dependents RSS < 50 MB with flag; RSS < 10 MB without.
- `go test ./cmd/bd/... -run TestShow` clean.

## Routing

- Builder bead: be-ijck6q → `beads/builder`
- Design bead be-775o60: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

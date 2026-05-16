# Plan — `bd list --json --limit 0` to IterIssues (be-c40q2v)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-c40q2v (designer, 2026-05-15)
**Parent architecture:** be-yinl4d (RAM budget — Iter migrations)
**Dependencies:** be-jaavsb (CLOSED), be-u0zlsq (CLOSED) — IterIssues available

## Goal

Eliminate the 13 GB memory regression on unbounded `bd list --json` calls by switching to
`IterIssues` when `IssueFilter.Limit == 0`. Output must be byte-identical to the slice path.

## Decomposition decision: one builder bead

Switch condition in `cmd/bd/list.go` + output framing + bench. Self-contained. One PR.

Builder bead: be-303d0l.

## Key design choice: switch threshold is exactly `Limit == 0`

No `DefaultLimit` override changes this. If `IssueFilter.Limit` is 0, use iterator.
If > 0, use `SearchIssues` (slice path, unchanged).

## Byte-identical output requirement

Output of the streaming path must be byte-identical to the slice path for bounded queries.
Existing list tests must pass without modification — they are the correctness oracle.

JSON framing: `[`, per-item `json.NewEncoder(w).Encode(item)` with leading comma after first, `]`.
`--indent`/`--no-indent` flags must keep working.

## Acceptance (builder bead be-303d0l)

- `bd list --json --limit 0` uses `IterIssues`.
- `bd list --json --limit N` (N > 0) unchanged (uses `SearchIssues`).
- Output byte-identical to slice path (existing list tests pass).
- Bench: RSS < 20 MB on 152-row fixture with `--label X --limit 0`.
- `go test ./cmd/bd/... -run TestList` clean.

## Routing

- Builder bead: be-303d0l → `beads/builder`
- Design bead be-c40q2v: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

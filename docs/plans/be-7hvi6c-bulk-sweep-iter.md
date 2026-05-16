# Plan — Bulk-sweep CLI callers to IterIssues (be-7hvi6c)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-7hvi6c (designer, 2026-05-15)
**Parent architecture:** be-yinl4d (RAM budget — Iter migrations)
**Dependencies:** be-jaavsb (CLOSED), be-u0zlsq (CLOSED) — IterIssues available

## Goal

Migrate 7 bulk-sweep call sites in `bd ado`, `bd doctor`, `bd find-duplicates`, `bd lint`,
and `bd label` from `SearchIssues` to `IterIssues`. Pure plumbing — no user-visible changes.

## Decomposition decision: one builder bead

7 mechanical substitutions, same migration pattern each time. Self-contained. One PR.

Builder bead: be-ty28x5.

## Call sites (exact file:line from design §2)

All 7 must be migrated:
1. `cmd/bd/ado.go:717` — `SearchIssues(ctx, "", IssueFilter{})` → `IterIssues`
2. `cmd/bd/ado.go:743` — `SearchIssues(ctx, "", IssueFilter{})` → `IterIssues`
3. `cmd/bd/ado.go:850` — `SearchIssues(ctx, "", IssueFilter{})` → `IterIssues`
4. `cmd/bd/doctor_pollution.go:25` — `SearchIssues(ctx, "", IssueFilter{})` → `IterIssues`
5. `cmd/bd/find_duplicates.go:111` — `SearchIssues` when `Limit == 0` → `IterIssues`
6. `cmd/bd/lint.go:92` — `SearchIssues` when `Limit == 0` → `IterIssues`
7. `cmd/bd/label.go:188` — `SearchIssues(ctx, "", types.IssueFilter{})` → `IterIssues`

Do NOT migrate: `label.go:274` (ParentID filter, bounded), `completions.go:46` (Limit:50),
`todo.go:102` (bounded by labels filter).

## Migration pattern

```go
it, err := store.IterIssues(ctx, "", types.IssueFilter{})
if err != nil { return err }
defer it.Close()
for it.Next(ctx) {
    issue := it.Value()
    // ... same body ...
}
if err := it.Err(); err != nil { return err }
```

In-memory aggregation maps (find-duplicates, doctor_pollution) are fine — only the input scan changes.

## Acceptance (builder bead be-ty28x5)

- All 7 call sites use `IterIssues` with `for it.Next() + defer it.Close() + it.Err()`.
- `go test ./cmd/bd/... -run 'TestAdo|TestDoctor|TestFindDup|TestLint|TestLabel'` clean.
- Bench: RSS < 100 MB per command on 100K-issue fixture.
- User-visible output unchanged.

## Routing

- Builder bead: be-ty28x5 → `beads/builder`
- Design bead be-7hvi6c: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

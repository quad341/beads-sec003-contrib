# Plan: bd stats --no-blocked (bd-f5zqs)

**PM:** beads/pm  
**Date:** 2026-05-17  
**Parent bead:** bd-f5zqs  
**Designer spec:** bd-f5zqs description field  

---

## Goal

`bd stats` is dominated by `computeBlockedIDs`, which takes ~2s on large rigs.
Add a `--no-blocked` flag that skips this traversal. Callers that don't need the
blocked count (CI dashboards, periodic reports) get sub-100ms stats immediately.

---

## Child beads

| Bead | Title | Routing | Blocks |
|------|-------|---------|--------|
| be-nnqp | Implement --no-blocked: flag, type change, human/JSON output | builder | be-zkzw |
| be-zkzw | Integration tests for --no-blocked behavior | validator | — |
| be-4b63 | Fix ui.RenderFail("0") on Blocked:0 (cosmetic bug) | builder | — |

---

## Dependency graph

```
bd-f5zqs (designer spec)
├── be-nnqp  [ready-to-build → builder]
│   └── be-zkzw  [needs-tests → validator, blocked by be-nnqp]
└── be-4b63  [ready-to-build → builder, standalone]
```

---

## be-nnqp — Implementation

**What changes:**

1. `internal/types/types.go` — `BlockedIssues int` → `BlockedIssues *int` (nullable JSON)
2. `cmd/bd/status.go` — add `BlockedCountSkipped bool` (omitempty) to `StatusOutput`
3. Add `--no-blocked` flag to `statusCmd`/`statsCmd` (after `--no-activity`, before `--json`)
4. When flag set: skip `computeBlockedIDs`, set `BlockedIssues = nil`, `BlockedCountSkipped = true`
5. Human output: `Blocked:         (skipped)` with `ui.MutedStyle` at column 32
6. Grep all `BlockedIssues` callers and add nil guards
7. Update long help text with new examples

**Acceptance:**
- `bd stats --no-blocked` < 100ms on 10k-bead corpus
- Default `bd stats` behavior unchanged
- `go build ./...` clean (all nil guards in place)

---

## be-zkzw — Tests

Blocked by be-nnqp. Write in `cmd/bd/status_embedded_test.go` (or existing stats test file).

Test cases:
1. `bd stats --no-blocked` human output contains `(skipped)`, no numeric Blocked value
2. `bd stats --no-blocked --json`: `blocked_issues` null, `blocked_count_skipped` true
3. `bd stats --json` (default): `blocked_issues` non-null, `blocked_count_skipped` absent
4. `bd stats` (default human): numeric Blocked value, no `(skipped)`
5. `bd stats --no-blocked --no-activity --json`: both flags compose correctly

Use `runCommandBuffers` pattern to avoid stderr leak into JSON parse.

---

## be-4b63 — Cosmetic bug (standalone)

`Blocked: 0` currently renders red via `ui.RenderFail`, implying a problem when none exists.

Fix: conditional in `cmd/bd/status.go` — render plain when count == 0, red when count > 0.

---

## Backward compatibility

- `blocked_issues: null` is valid JSON; callers doing `obj.blocked_issues || 0` degrade gracefully
- `blocked_count_skipped` is omitempty — default output is structurally unchanged
- Human output table width/column alignment unchanged

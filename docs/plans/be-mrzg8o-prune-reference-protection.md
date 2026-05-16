# Plan — `bd prune` reference protection (be-mrzg8o)

**PM:** beads/pm · **Date:** 2026-05-07
**Parent architecture:** be-5sn (architect) · **Design:** be-mrzg8o.design (designer)
**Related:** be-tc3wh3 (bench, separate bead) · be-yinl4d (iterator API, closed)

## Goal

Ship reference-aware `bd prune`: closed beads cited by ID in any
non-closed bead's description, notes, or comments are protected from
deletion. New `--ignore-references` flag is the explicit override.

## Context

The 2026-05-01 incident pruned architect decision beads (be-08pl, be-eei)
that were still cited by open verification beads. Substance survived only
because the architect rig keeps local Markdown copies. be-5sn locks in
the option-(b) reference-aware mechanism; the design (be-mrzg8o.design)
prescribes exact placement, signatures, CLI surface, and 8 test cases.

## PM judgements (designer escalations)

The designer flagged two judgements for PM review. Both **approved**:

### (a) Include `referenced_ids_sample` in actual-prune JSON, not just dry-run — APPROVED

be-5sn §9 originally scoped the sample to dry-run output. The designer
extended it to actual-prune output too. PM agrees:

- The sample IDs reference beads that were *skipped* (still alive), not
  deleted — there is no dangling-reference risk.
- Operators auditing post-prune output (CI logs, dashboards) benefit
  from seeing what got preserved.
- The sample is capped at 100 IDs; output size stays bounded.
- The cost of including it is one additional JSON field per actual-prune
  invocation. Negligible.

### (b) Fix existing `pinned_skipped` silent-drop in empty-result branch in the same PR — APPROVED

Today, when `bd prune` finds no candidates, it prints "No closed beads
to prune" and silently drops `pinned_skipped` info. With
`referenced_skipped` joining the family, the silent drop becomes a
visible inconsistency. The fix mirrors the dry-run output format — small
addition, tightly coupled to the new feature. Bundling it in the same
PR keeps the surface coherent.

## Decomposition decision: one builder bead

The architect (be-5sn §16) decomposed the architecture into three beads:

1. **Implementation** (this PM cycle's input — be-mrzg8o)
2. **Bench** — be-tc3wh3, separate (still awaiting designer pickup)
3. **Iterator migration** — waits on be-yinl4d, separate

The implementation itself is a single coherent unit:

- One new file (`cmd/bd/prune_refs.go`, ~80 LOC)
- Targeted edits to two existing files (`purge.go`, `prune.go`)
- 8 tests (T1-T7 embedded + T8 unit) — code already specified in the
  design doc, essentially transcription
- Test helpers — small, isolated additions

**One builder bead** ships the full feature:

- The architect's §16 explicitly noted "Builder can ship them as one PR
  if scope is small."
- Splitting impl from tests creates synthetic ordering with no value —
  test code is already authored in the design.
- Tests reference symbols only the implementation creates; co-location
  reduces churn.
- Single PR review surface is more coherent for a focused safety
  feature.

The bench (be-tc3wh3) and iterator migration remain separate, as already
intended.

## Acceptance criteria (carried from design §6)

The builder bead's done-when checklist:

- [ ] `cmd/bd/prune_refs.go` exists with `buildReferencedSet` +
      `scanText` + `referencedSampleCap = 100`
- [ ] `cmd/bd/purge.go::runPurgeOrPrune` has the §2.1 insertion stub
      (after pinned filter, before nothing-to-do branch)
- [ ] `cmd/bd/prune.go` registers `--ignore-references` (default false)
      and updates Long-help text per §4.2 + EXAMPLES line per §4.2
- [ ] Empty-result branch reports `pinned_skipped` and
      `referenced_skipped` in human + JSON output (§4.7) — PM-approved
      bundling of pre-existing UX bug fix
- [ ] Sample-line rule at the dry-run print site: 5 inline IDs +
      `(+N more)` suffix when count > 5; line omitted when count == 0
- [ ] `--json` output for both dry-run AND actual-prune carries
      `referenced_skipped`, `referenced_count`, and (when count > 0)
      `referenced_ids_sample` — PM-approved per judgement (a)
- [ ] `cmd/bd/prune_refs_embedded_test.go` carries T1-T7 with the
      `cgo && dolt_only` build tag
- [ ] `cmd/bd/prune_refs_test.go` carries the unit test T8 (no build
      tag)
- [ ] Test helpers (`createAndClosePinned`, `createOpenWithBody`,
      `requireBeadExists`, `requireBeadAbsent`, `requireJSONField`,
      `requireJSONFieldAbsent`, `requireJSONIDSampleContains`) live in
      `cmd/bd/prune_test_helpers.go` (or `bd_test.go` if more natural)
- [ ] `go test ./cmd/bd/...` clean
- [ ] Manual repro: in a rig with the be-yinl4d ADR pattern, prune does
      not delete the architect bead while children exist (the canonical
      case from be-5sn §1)

## Guardrails (from architecture + design — DO NOT relitigate)

- Use `Statuses` filter, NOT `ExcludeStatus` (PG silently drops the
  latter — be-jdeief gap). See design §3.1.
- Do NOT add the scan to `bd purge`. be-5sn §15.
- Do NOT scan `title` or `metadata`. be-5sn §11.
- Do NOT cache `refSet` between runs. References change as beads are
  written. be-5sn §15.
- `--ignore-references` defaults to `false`. Loud opt-in only.
- Do NOT pre-migrate to `IterIssues` / `IterIssueComments`. Slice path
  is acceptable now; migration is a separate bead post-be-yinl4d.

## Out of scope

- Bench (be-tc3wh3) — separate bead, still awaiting designer
- Iterator migration — separate bead, awaiting be-yinl4d
- Cross-rig reference detection — known limitation per be-5sn §11
- A `bd refs <id>` query view — separable feature
- Title- or metadata-based reference detection — be-5sn §11

## Routing

- 1 child bead (`ready-to-build`) → `beads/builder`

Bench (be-tc3wh3) routes itself when picked up by designer; not part of
this PM cycle.

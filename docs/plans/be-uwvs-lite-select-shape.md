# Plan: be-uwvs handoff — Lite SELECT shape (`IssueFilter.Lite`)

**Source design:** be-uwvs (designer, 2026-05-07)
**Architecture parent:** be-jp5s (architect, closed)
**Investigation:** be-81n
**Sibling work:** be-qmvl (application-layer shallow JSON; provides `--full` flag at application layer)
**Sequence dep:** be-tjiy (PG filter-drop bug — closed; cleared to proceed)
**Owner of plan:** beads/pm
**Date:** 2026-05-07

## Goal

Add a "lite" SELECT shape to the storage layer so internal callers
(routing + listing paths) skip materializing heavy TEXT columns
(`description, design, acceptance_criteria, notes, payload, waiters`).
Defaults preserve today's behavior; opt-in per filter via
`IssueFilter.Lite`. Wire on `bd list --json` and three siblings.
Composes with be-qmvl (application-layer shallow JSON via shared
`--full` flag).

## PM decisions (designer §9 open questions)

1. **Persistent flag?** No — `--full` lives **locally** on `bd list`,
   `bd ready`, `bd dep tree`, `bd graph`. Designer's recommendation
   accepted. Avoids polluting 50+ commands with an irrelevant flag.
2. **CHANGELOG classification.** Minor version bump. Entry under
   "Behavior changes" — `bd list --json` (and three siblings) now omit
   heavy text fields by default; `--full` restores the previous
   payload.
3. **`bd ready --claim`.** Option A — `--claim` always returns full
   payload regardless of the lite default. Builder pattern: claim path
   sets `Lite=false` explicitly. Documented in `bd ready --help`.
4. **`WorkFilter.Lite`.** In scope — mirror onto `WorkFilter.Lite` so
   `bd ready --json` (the hot path) gets SQL-layer lite. **Fallback if
   per-backend routing is painful:** drop SQL-layer lite for `bd ready`
   and rely on be-qmvl shallow JSON only. This is acceptable —
   designer flagged it as a nice-to-have, not a blocker. Builder makes
   the call during implementation. Document the chosen path in the bead.

## Decomposition

5 child beads. Three builder, two validator.

| Child | Routing | Title | Blocked-by |
|-------|---------|-------|------------|
| **A** | builder | Foundation: `IssueSelectColumnsLite`, scan helper, filter field, parity test | — |
| **B** | builder | CLI wiring: `bd list/dep tree/graph --json` default lite; `--full` flag composition | A |
| **C** | builder | `WorkFilter.Lite` mirror + `bd ready --json` lite (with fallback) | A |
| **D** | validator | Per-backend lite-correctness tests + PG label-filter regression + benchmark | A |
| **E** | validator | Export/migration filter spy gates (4 sites) | A |

### A — Foundation (storage layer)

Single PR. Pure storage-layer change with zero behavior diff at the
default (`Lite=false`):

- New constant `IssueSelectColumnsLite` in
  `internal/storage/issueops/scan.go` per designer §1.1.
- New slice `HeavyDropList` (test-only, not used by production
  callers).
- New scan helper `ScanIssueLiteFrom` per designer §1.2 — sets
  `IsLitePartial=true`.
- New field `types.Issue.IsLitePartial bool` (`json:"-"`) per
  designer §1.3.
- New field `types.IssueFilter.Lite bool` per designer §1.4.
- `searchTableInTx` switches SELECT shape + scanner on `filter.Lite`
  per designer §1.5.
- Schema-parity test
  `TestIssueSelectColumns_LitePlusHeavyEqualsFull` per designer §5
  (with the actionable error message).
- godoc on every new symbol.
- `EXTENDING.md` entry: caller contract for `IsLitePartial` per
  designer §4.

**Out of scope for A:** any CLI change (B), any cross-backend correctness
gate (D), any export gate (E).

### B — CLI wiring + `--full` flag composition

Single PR. Depends on A. Coordinate with be-qmvl (`--full` at
application layer):

- `bd list --json` defaults to `filter.Lite=true`; existing/new
  `--full` flag flips to `false` AND disables shallow JSON.
- `bd dep tree --json`, `bd graph --json` likewise default lite.
- `--full` flag wired locally on `bd list`, `bd dep tree`, `bd graph`.
  Help text per designer §3.2 (mention heavy fields and that the
  default is lite).
- `bd show <id>` is **unchanged** — full payload always.
- CHANGELOG.md entry under "Behavior changes" with the tagline
  designer suggested in §9.2.

**`--full` composition with be-qmvl:**
- If be-qmvl ships first (expected), this bead extends `--full` to
  also flip `filter.Lite`.
- If this bead ships first, introduce `--full` with both behaviors
  in this bead.
- Builder reads the merge order at implementation time.

### C — `WorkFilter.Lite` mirror + `bd ready --json`

Single PR. Depends on A. Treats designer's fallback as legitimate:

- Add `WorkFilter.Lite bool` mirroring `IssueFilter.Lite`.
- Per-backend `GetReadyWork` impls (PG / Dolt / embedded) thread
  Lite into the underlying `searchTableInTx` (or equivalent) so
  lite output matches across backends.
- `bd ready --json` defaults `WorkFilter.Lite=true`.
- `bd ready --claim` sets `Lite=false` (Option A from designer §2.1).
- `bd ready --explain` keeps full payload.
- `bd ready --json --full` flips to `false` end-to-end.
- **Fallback path:** if per-backend routing is impractical, drop
  SQL-layer lite for `bd ready` and rely on be-qmvl shallow only.
  Document the chosen path in the closing comment.

### D — Per-backend lite tests + benchmark (validator)

- `TestSearchIssues_Lite_BlankHeavyFields` per backend (PG / Dolt /
  embedded), per designer §5.1. Insert fixture with non-empty heavy
  fields; run with `Lite=true`; assert heavy fields blank,
  `IsLitePartial=true`, identity/metadata/labels preserved. Compare
  inverse with `Lite=false`.
- `TestSearchIssues_Lite_PG_LabelFilter` per designer §5.2 — guards
  against be-tjiy regression. Two labelled, two unlabelled fixtures;
  `Labels=["X"] Lite=true`; assert exactly two rows.
- Add lite-mode case to existing PG/Dolt/embedded parity matrix —
  byte-identical JSON across backends on the same fixture.
- Benchmark per designer §10 (updated per architect decision be-lol7):
  1k issues with 50KB description each. Acceptance (dual-gate, both
  enforced as hard CI gates in `TestLiteSearchAllocReduction`):
  - **Primary gate:** ≥90% B/op reduction under lite vs full
    (`liteBytesPerOp / fullBytesPerOp ≤ 0.10`).
  - **Sanity gate:** ≥15% per-row allocs/op reduction
    (`liteAllocsPerRow / fullAllocsPerRow ≤ 0.85`), confirming the
    lite SELECT branch is active.

### E — Export/migration filter spy gates (validator)

Per designer §6. Single PR. Add a spy storage type that captures the
last `IssueFilter` it saw, then assert at 4 sites:

- `cmd/bd/export_test.go` — `TestExportFilterIsAlwaysFull`: assert
  `Lite=false`, `Limit=0`.
- `cmd/bd/export_auto_test.go` — same shape.
- `cmd/bd/migrate_issues_test.go` — same shape.
- `cmd/bd/jira_test.go` — same shape.

**Note:** be-x42v's parallel test gate adds `MaxRows=0` to the same
spy. Whichever lands first creates the spy infrastructure; the second
adds the field. Validator reconciles at impl time.

## Risks (carried from architecture be-jp5s)

- **R-07 — PG filter-drop interaction.** be-tjiy is closed; bead D's
  PG label-filter regression test gates regressions.
- **JSON consumer surprise.** Lite output changes the shape of
  `bd list --json` etc. — heavy fields absent by default.
  Mitigation: CHANGELOG entry, `--full` flag with discoverable help.
- **Builder convenience hit.** `bd ready --json` consumers that
  read description on the listing path will need `--full` or a
  follow-up `bd show`. Acceptable per designer §2 — most listing
  consumers don't read body.

## Done-when (rolls up to root be-uwvs)

- All 5 child beads closed.
- Schema-parity test in CI.
- Per-backend lite-correctness tests in CI.
- `bd list --json | jq '.[].description'` returns `null` (lite
  default); `bd list --json --full | jq '.[].description'` returns
  populated body.
- `bd ready --claim --json` returns full payload (regardless of
  default).
- `bd export | wc -l` matches today's output (lossless round-trip).
- Benchmark documented; dual CI gates pass: ≥90% B/op reduction (primary) and ≥15% allocs/op reduction (sanity) in `TestLiteSearchAllocReduction`.
- CHANGELOG entry merged.

## Refs

- Designer notes: be-uwvs notes section (full spec, §1–§11)
- Architecture: be-jp5s
- Sibling: be-qmvl (`--full` at application layer)
- Sequence: be-tjiy (PG filter-drop bug, closed)

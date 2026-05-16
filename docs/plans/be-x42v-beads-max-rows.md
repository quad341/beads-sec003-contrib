# Plan: be-x42v handoff — `BEADS_MAX_ROWS` + `--max-rows` flag

**Source design:** be-x42v (designer, 2026-05-07)
**Architecture parent:** be-jp5s (architect, closed)
**Investigation:** be-81n
**Owner of plan:** beads/pm
**Date:** 2026-05-07

## Goal

Add a process-level row-count safety net to bd's `SearchIssues` path.
Operator opts in via env var `BEADS_MAX_ROWS=N` or per-invocation flag
`--max-rows N`. Default off (no behavior change). On overage, return
typed error → exit code 2 with two-line stderr message; stdout stays
empty.

## PM decisions (designer §9 open questions)

1. **`bd config show` env-var listing.** Confirmed: builder must
   verify `BEADS_MAX_ROWS` shows up alongside `BEADS_DIR` etc. If
   `cmd/bd/config_show.go` doesn't list it after impl, that's a defect
   to fix in the same bead.
2. **Settable via `bd config set`?** No. It's an ops/CI shell-session
   safety knob, not a per-project preference. Setting via
   `.beads/config.yaml` would defeat the "operator opts in per shell"
   intent. Designer's recommendation accepted.
3. **CHANGELOG visibility.** Confirmed: prominent entry under
   "Operations / Hardening" with the tagline designer suggested in
   §9.3.

## Decomposition

4 child beads. Two builder, two validator.

| Child | Routing | Title | Blocked-by |
|-------|---------|-------|------------|
| **1** | builder | Storage foundation: `IssueFilter.MaxRows`, `MaxRowsSource`, `ErrTooManyRows`, `searchTableInTx` integration | — |
| **2** | builder | CLI wiring: `--max-rows` flag, `BEADS_MAX_ROWS` env, error handling, doctor-family env opt-in, CHANGELOG, CONTRIBUTING.md | 1 |
| **3** | validator | CLI behavioral tests + per-backend storage parity tests | 1, 2 |
| **4** | validator | Test gates for opt-out commands (export / export --auto / migrate-issues / jira) — filter spy + env-bypass behavioral test | 2 |

### 1 — Storage foundation (builder)

Single PR. Pure storage-layer change with zero behavior diff at the
default (`MaxRows=0`):

- New fields on `IssueFilter`: `MaxRows int`, `MaxRowsSource string`
  per designer §3.1.
- New typed error `*issueops.ErrTooManyRows` in
  `internal/storage/issueops/errors.go` per designer §3.2.
- `searchTableInTx` integration per designer §3.3:
  - LIMIT computation:
    `effectiveLimit = max( min(Limit,MaxRows+1), MaxRows+1 )` when
    `MaxRows > 0` (or simpler equivalent — see designer pseudocode).
  - Post-scan check: if `len(issues) > MaxRows`, return
    `ErrTooManyRows{Found, Cap, Source}`.
- Apply same pattern to PG and Dolt backend equivalents so cap fires
  uniformly across backends.
- Storage-layer happy-path unit test: `Lite=false MaxRows=N` over a
  fixture with `> N` rows returns `ErrTooManyRows` with correct
  `Found` and `Cap`.

**Out of scope for 1:** any CLI change (2), per-backend parity (3),
opt-out test gates (4).

### 2 — CLI wiring (builder)

Single PR. Depends on 1. Per designer §3.4 + §4 + §5:

- `--max-rows int` flag (negative rejected with exit 1) on:
  `bd list`, `bd ready`, `bd dep tree`, `bd find-duplicates`,
  `bd graph`. Help text per designer §2.2.
- `BEADS_MAX_ROWS` env var read on the 5 above + on
  `bd doctor`, `bd lint`, `bd doctor-conventions`,
  `bd doctor-pollution`. Bad env value → warning to stderr, ignore
  (proceed disabled). Per designer §3.4.
- Resolution helper that picks flag > env > 0; sets
  `filter.MaxRowsSource` to `"--max-rows"` or `"BEADS_MAX_ROWS"`
  accordingly.
- Error catching: `errors.As` for `*ErrTooManyRows`; emit two-line
  stderr message per designer §2.3 (no ANSI color); exit 2; stdout
  empty.
- Negative flag handling: `FatalError("--max-rows must be
  non-negative; got %d", n)` → exit 1.
- `bd config show` lists `BEADS_MAX_ROWS` if set; verify in
  `cmd/bd/config_show.go`.
- CHANGELOG.md entry under "Operations / Hardening" with
  designer's §9.3 tagline.
- `cmd/bd/CONTRIBUTING.md` (or equivalent — builder picks site)
  documents the rule: every `IssueFilter`-using site outside the
  designer §4 matrix must explicitly initialize `filter.MaxRows = 0`
  in code.

### 3 — Behavioral + storage tests (validator)

Single PR. Depends on 1 + 2.

CLI tests in `cmd/bd/max_rows_test.go` per designer §6.1 — 10
scenarios:
- `TestMaxRows_Disabled_NoEnv`
- `TestMaxRows_Flag_UnderCap`
- `TestMaxRows_Flag_OverCap`
- `TestMaxRows_Env_OverCap`
- `TestMaxRows_Flag_OverridesEnv`
- `TestMaxRows_Flag_Zero_OverridesEnv`
- `TestMaxRows_BadEnv_LogsAndIgnores`
- `TestMaxRows_Negative_FlagRejected`
- `TestMaxRows_LimitSet_CapTighter`
- `TestMaxRows_LimitSet_CapLooser`

Storage tests in `internal/storage/issueops/search_test.go` per
designer §6.2:
- `TestSearchIssues_MaxRows_NotExceeded`
- `TestSearchIssues_MaxRows_Exceeded_ReturnsErrTooManyRows`
- `TestSearchIssues_MaxRows_Zero_NoCap`
- `TestSearchIssues_MaxRows_WithLimit`
- `TestSearchIssues_MaxRows_BackendParity` (PG / Dolt / embedded —
  fold into existing parity framework if present)

### 4 — Opt-out command test gates (validator)

Single PR. Depends on 2.

Filter spy (`filterCapturingStore`) per designer §6.3 — assert
`MaxRows=0`, `MaxRowsSource=""`, `Lite=false`, `Limit=0` at:

- `cmd/bd/export_test.go`
- `cmd/bd/export_auto_test.go`
- `cmd/bd/migrate_issues_test.go`
- `cmd/bd/jira_test.go`

Plus env-bypass behavioral test per designer §6.4:
`TestExport_BypassesBeadsMaxRows` — `t.Setenv("BEADS_MAX_ROWS", "1")`
on a 5-issue fixture; assert export returns all 5 rows.

**Note:** be-uwvs's parallel export gate (its bead E) asserts
`Lite=false` on the same spy. Whichever bead lands first creates the
spy infrastructure; the second adds the additional field assertions.
Validator reconciles at impl time.

## Risks (carried from architecture be-jp5s)

- **R-03 — Env-var inheritance visibility.** A parent shell setting
  `BEADS_MAX_ROWS` silently affects child bd subprocesses.
  Mitigations: error message names env var explicitly; `bd config
  show` lists it. Designer §7 elaborates.
- **Interaction with `--limit 0` semantics.** `--limit 0` still means
  unlimited at the contract level; the cap is independent.
  `(Limit=100, MaxRows=5)` errors when matches > 5 even though
  Limit > MaxRows — by design (cap is a defensive wall, not a
  truncation). Test `TestMaxRows_LimitSet_CapTighter` gates this.
- **Half-rendered JSON.** stdout MUST stay empty when cap fires;
  designer §2.3 explicit. Builder ensures CLI catches the error
  before any rows are streamed to stdout.

## Done-when (rolls up to root be-x42v)

- All 4 child beads closed.
- `BEADS_MAX_ROWS=5 bd list --all --limit 0` exits 2 with two-line
  stderr message (env source).
- `bd list --max-rows 5` exits 2 with flag source named in error.
- `bd list --max-rows 0` (or unset env) is byte-identical to today.
- `bd list --max-rows -1` exits 1 with usage error.
- `BEADS_MAX_ROWS=banana bd list --json` warns to stderr,
  exits 0, JSON unchanged.
- `BEADS_MAX_ROWS=1 bd export -o issues.jsonl` exports all rows
  (test gate + behavioral test).
- `bd config show` lists `BEADS_MAX_ROWS` when set.
- Per-backend SearchIssues parity covers MaxRows.
- CHANGELOG entry under "Operations / Hardening" merged.

## Refs

- Designer notes: be-x42v notes section (full spec, §1–§11)
- Architecture: be-jp5s (R-03 env inheritance)
- Sibling: be-uwvs (lite SELECT shape; composes via shared
  `IssueFilter`)

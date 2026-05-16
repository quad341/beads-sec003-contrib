# be-phqjc0 — bead-vanishing P0: rebase + audit + repro plan

**Root bead:** be-phqjc0 (P0, bug, assigned mayor → pm)
**Filed:** 2026-05-12 by mayor-adhoc-9e2c5d6c53
**Driver:** Bead-vanishing is now commonplace. Today's recurrences (be-0dej standup vanished in 5 min, be-lol7 vanished mid-routing in beads/designer) block factory autonomy.

## Branch state (confirmed)

- Target branch: `fix/be-j72zr5-not-implemented-bead-ref`
- HEAD: `7562bcfd5 fix(postgres/errors): be-j72zr5 retarget errNotImplemented at be-21updd`
- Gap: **87 commits in `origin/main` not on fix branch**
- Origin HEAD: `da73b7511 Merge pull request #3889 from coffeegoddd/db/split-writes` — split-writes merge is suggestive: write-path changes could be the vanish fix.

## Constraints

- **PG-backend is LOCAL-ONLY** per mayor scope 2026-05-02 (see memory key `pg-backend-be-6fk-epic-is-local-only`). Rebase brings upstream IN; nothing pushes OUT until benchmarks justify (see be-nwwe75).
- Vanishing observed in BOTH dolt-backed and PG-backed rigs (be-0dej vanished in beads rig today, dolt-backed). Root cause may not be PG-specific.
- This decomposition itself is at risk: child beads filed below could vanish. Mitigations:
  - Plan persisted in this file (worktree-local, untracked — but stable on disk)
  - Child IDs mailed to mayor immediately after creation
  - Plan snapshot saved as bd memory key

## Children (5 beads)

```
be-phqjc0 (root)
├── AUDIT      — upstream PR audit (builder)               [no deps]
├── REPRO      — vanish-repro script authoring (validator) [no deps]
├── BASELINE   — pre-rebase repro run (validator)          [depends REPRO]
├── REBASE     — 87-commit rebase (builder)                [depends AUDIT]
└── VERIFY     — post-rebase repro + verdict (validator)   [depends REBASE + BASELINE]
```

### 1. AUDIT — Upstream PR audit for vanish fixes (builder)

Read all 87 upstream commits and identify any that fix bead-vanishing, sync races, or write-path data loss.

**Targets:**
- Grep PR titles + bodies since merge-base for: `vanish`, `missing`, `lost`, `sync.*race`, `duplicate.*id`, `transaction`, `rollback`, `split-writes`, `embedded`, `dolt.*conn`
- Read carefully: #3498-revert chain (#3498, #3888, #3890, #3891, #3892) — pattern of revert+re-add suggests data-loss bug
- Read carefully: #3889 split-writes merge (upstream HEAD) — write-path changes could be the fix
- Read carefully: ba2873249 `change max conns` and 9cdb62868 dolt driver bump (1.86.4→1.88.1)

**Output:** `docs/plans/be-phqjc0-upstream-audit.md` (in builder worktree) with:
- Table of candidate fixes (commit, PR#, title, relevance)
- Verdict line: either `UPSTREAM-FIX-AVAILABLE: <commit/PR>` or `NO-UPSTREAM-FIX; rebase needed for general hygiene, vanish root-cause investigation must follow`

**Acceptance:**
- Findings doc exists
- All 5 commits in #3498-revert chain explicitly evaluated
- Verdict is explicit (not "maybe")
- Mail summary to pm with verdict

### 2. REPRO — Vanish-repro script (validator)

Author a deterministic script that reproduces the vanish phenomenon.

**Spec:**
- `scripts/repro-vanish.sh` (or similar location validator chooses)
- Creates N beads (default N=10), tags them with run ID
- Immediately polls `bd show <id>` and `bd list | grep <run-id>` at 1s/5s/30s/300s after each create
- Captures: vanish rate (N gone / N created), vanish timing distribution
- Env switches for backend: `BEADS_BACKEND=dolt-embedded` vs `BEADS_BACKEND=postgres`
- Outputs JSON line per bead: `{id, created_at, last_seen_at, vanish_window_seconds, present_at_300s}`
- Exit code: 0 if zero vanishes, 1 otherwise

**Acceptance:**
- Script lives in repo (committed)
- README section explains invocation + expected output
- Smoke test: script runs cleanly on builder's worktree (validator runs it once to verify functionality, not yet for diagnostic data)
- Mail summary to pm with path to script

### 3. BASELINE — Pre-rebase baseline (validator, depends REPRO)

Run the repro on current HEAD (pre-rebase) to establish quantitative baseline.

**Spec:**
- Run on `fix/be-j72zr5-not-implemented-bead-ref` HEAD `7562bcfd5`
- N=20 beads minimum
- Run twice: once with dolt-embedded, once with postgres
- Total: 40 repro samples per backend (combined N>=40 helps wash transient noise)

**Acceptance:**
- Baseline artifact: `docs/plans/be-phqjc0-baseline.md` with summary table + raw JSON results attached
- Records: backend, N, vanish_count, vanish_rate, median vanish window
- Mail summary to pm

### 4. REBASE — Merge/rebase 87 commits (builder, depends AUDIT)

Bring `fix/be-j72zr5-not-implemented-bead-ref` up to `origin/main`.

**Approach (builder picks):**
- Prefer `git merge origin/main` over interactive rebase, to preserve PG-backend stack commit history
- Resolve conflicts preserving PG-backend changes (anything under `internal/storage/postgres/`, `cmd/bd/init_postgres.go`, `cmd/bd/store_factory.go`)
- Run `go build ./...` + `go test ./...` after merge — must pass

**Acceptance:**
- Branch fast-forward includes `origin/main` HEAD (`da73b7511` or newer)
- `go test ./...` passes
- Any new conflicts/decisions documented in merge commit message
- Branch is **NOT pushed upstream** (PG-backend stays LOCAL-ONLY per be-nwwe75)
- Mail summary to pm with merge commit SHA + test results

### 5. VERIFY — Post-rebase repro + verdict (validator, depends REBASE + BASELINE)

Re-run repro on rebased branch and compare to baseline.

**Spec:**
- Run repro (same params as BASELINE: N=20, both backends)
- Compare to BASELINE artifact

**Acceptance:**
- Artifact: `docs/plans/be-phqjc0-verdict.md` with before/after table
- If post-rebase vanish_rate == 0 on both backends: **PASS** — close root bead, save the fix as memory
- If post-rebase vanish_rate > 0 on either backend: **FAIL** — file new investigation bead routed to architect with:
  - Title: "Vanish root cause investigation — upstream rebase did not fix"
  - Label: `needs-architecture`
  - Includes: baseline, post-rebase, AUDIT findings
  - Memory hint: "bd embedded vs city-shared dolt sync state" (from vanish memory)
- Mail summary to pm + mayor

## Routing summary

| Child   | Agent     | Label             | Metadata                       | Blocked by      |
|---------|-----------|-------------------|--------------------------------|------------------|
| AUDIT   | builder   | ready-to-build    | gc.routed_to=beads/builder     | (none)          |
| REPRO   | validator | needs-tests       | gc.routed_to=beads/validator   | (none)          |
| BASELINE| validator | needs-tests       | gc.routed_to=beads/validator   | REPRO           |
| REBASE  | builder   | ready-to-build    | gc.routed_to=beads/builder     | AUDIT           |
| VERIFY  | validator | needs-tests       | gc.routed_to=beads/validator   | REBASE, BASELINE|

## Risk: this plan can vanish

Per memory `bead-vanishing-phenomenon-2026-05-11-beads-created`, beads created today have a non-trivial chance of becoming unfindable in the same session. Mitigations applied:

1. This plan doc on disk (you're reading it)
2. Mail to mayor with all 5 child IDs + slug-by-slug summary (sent in workflow step after creation)
3. `bd remember` key `be-phqjc0-decomposition-child-ids-2026-05-12` recording all child IDs
4. `gc sling` each child immediately so the agent session catches it

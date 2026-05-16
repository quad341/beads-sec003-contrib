# Plan: be-crd handoff — bd 120s timeouts, five-slice mitigation

**Source architecture:** be-crd (architect, 2026-04-25; closed)
**Per-slice architect specs:** be-695, be-w3n, be-4v2, be-hjj, be-tve
**Per-slice designer notes:** in NOTES on each of the same beads
**Owner of plan:** beads/pm
**Date:** 2026-04-26

## Goal

Land the five-slice mitigation for "bd list timed out after 120s"
that the architect designed in be-crd and the designer scoped on
each child bead. The mitigations are:

1. **AD-01 — Defense-in-depth test isolation** in beads.
2. **AD-02 — `bd list --skip-labels` flag** (renamed from architect's `--no-labels` to avoid an existing-flag collision; see decision below).
3. **AD-03 — Reconciler load-shaping** in gascity (stagger + adaptive cadence + skip-labels caller).
4. **AD-04 — Operational Dolt cleanup formula hardening** (port resolution, purge, orphan reaper, schedule).
5. **AD-05 — bd-wrapper resilience** in gascity (`bd.slow` telemetry + bounded retry on idempotent reads).

Goals overlap and compound: leaked test load + retained garbage
raises the floor of Dolt latency; per-agent fan-out raises the
ceiling of concurrent demand; the IN-clause shape exacerbates
contention; the 120 s wrapper turns latency tail-spikes into hard
failures. Fixing any one helps; fixing them in concert eliminates
the symptom.

## Decomposition

The architect and designer have done all up-front design work.
Each slice has a clear architect spec + designer wireframes /
text artifacts inline on the source bead. The pm decomposition is
small — turning architect's "Files in scope" + designer's
implementation slice list into actionable build beads with crisp
acceptance criteria.

Total: 11 child build beads across 5 slices. All routed to
`beads/builder` with label `ready-to-build`.

| Slice | Source bead | Children | Notes |
|-------|-------------|----------|-------|
| AD-01 | be-695 (P1, independent) | be-c5p (P1 core), be-xtf (P2 docs) | 2 children |
| AD-02 | be-w3n (P1, independent) | be-a5z (P1) | 1 child — single-PR scope |
| AD-03 | be-4v2 (P1, depends on AD-02) | be-9u4 (P1, blocked-by be-a5z), be-hk6 (P1), be-1ya (P1) | 3 children — naturally split |
| AD-04 | be-hjj (P0, independent) | be-o9z (P0), be-4r4 (P0, blocked-by be-o9z), be-avn (P1) | 3 children — most operationally impactful |
| AD-05 | be-tve (P2, independent) | be-3fp (P2), be-ps0 (P2, blocked-by be-3fp) | 2 children |

### Per-slice details

#### AD-01 — Defense-in-depth test isolation (be-695, P1)

| Child | Title | Routes | Blocked-by |
|-------|-------|--------|------------|
| be-c5p (P1) | Defense-in-depth test isolation: isProductionPort + DB-name firewall | beads/builder | — |
| be-xtf (P2) | README: test-isolation env vars section | beads/builder | — |

**Why two beads.** be-c5p is the implementation core (port-resolution helper, panic + firewall message updates, bench-harness compatibility, tests). It's tightly coupled and forms one PR. be-xtf is a small docs slice that can land independently before, with, or after — splitting it out lets the docs go first if a writer is available.

**Coordination point.** be-c5p extends `internal/storage/dolt/store.go:54-72`'s `testDatabasePrefixes` to include `benchdb_`. The sister bead **be-avn** (under be-hjj) syncs `cmd/bd/dolt.go:staleDatabasePrefixes` against the same target list. After both land, the firewall list, the cleanup list, and the formula list converge.

#### AD-02 — `bd list --skip-labels` flag (be-w3n, P1)

| Child | Title | Routes | Blocked-by |
|-------|-------|--------|------------|
| be-a5z (P1) | bd list --skip-labels flag (AD-02 full implementation) | beads/builder | — |

**Why one bead.** Flag wiring + conflict validation + query gate + JSON shape + footer note + help text + tests all gate the same PR. Splitting would produce orphan beads waiting on each other for evidence one PR must bundle anyway. Same logic as the be-eei-d4v2 plan — bundle when deliverables share a PR's gates.

**Critical naming decision (designer).** Architect proposed `--no-labels`; that flag name is already used as a *filter* on `bd list`. The designer recommended `--skip-labels` (action verb, distinct from filter, no rename of the existing flag). All downstream beads use `--skip-labels`. Builder confirms the rename with the latest be-a5z status before starting the gascity-side plumbing in be-9u4.

#### AD-03 — Reconciler load-shaping (be-4v2, P1)

| Child | Title | Routes | Blocked-by |
|-------|-------|--------|------------|
| be-9u4 (P1) | Reconciler skip-labels plumbing in gascity | beads/builder | be-a5z (AD-02) |
| be-hk6 (P1) | Deterministic reconciler stagger | beads/builder | — |
| be-1ya (P1) | Adaptive cadence + telemetry in gascity reconciler | beads/builder | — |

**Why three beads.** The architect's three sub-decisions (§3.1 stagger, §3.2 adaptive cadence, §3.3 skip-labels caller) are naturally separable: they touch different code paths and have different blockers. Splitting lets stagger and cadence land while be-9u4 waits on the bd-side flag (be-a5z).

**Highest-risk gate (be-9u4).** The `beadChanged` correctness fix when labels are skipped is the load-bearing change. Without it, the reconciler will fire ~4500 spurious `bead.updated` events per minute. The validator should weight this regression test (designer §5.4) heavily during review.

#### AD-04 — Operational Dolt cleanup formula hardening (be-hjj, P0)

| Child | Title | Routes | Blocked-by |
|-------|-------|--------|------------|
| be-o9z (P0) | gc dolt cleanup: port resolution + purge + orphan reaper (core) | beads/builder | — |
| be-4r4 (P0) | Formula update + nightly order (mol-dog-stale-db) | beads/builder | be-o9z |
| be-avn (P1) | Sync bd staleDatabasePrefixes with formula list | beads/builder | — |

**Why three beads.** The architect's spec splits naturally into three: the Go-side `gc dolt cleanup` enhancement (be-o9z); the formula + scheduled order that shells out to it (be-4r4); and the bd-side prefix-list sync (be-avn). The sequencing is be-o9z → be-4r4. be-avn is independent — small follow-up that pairs with be-c5p (AD-01) for prefix-list convergence.

**Cross-repo work.** be-o9z and be-4r4 modify `gc-management` (the gc CLI + formula/order TOMLs). be-avn modifies `beads`. The builder may need cross-repo access; if not, escalate to architect.

**Operational priority.** This slice is P0 because it reclaims ~9.3 GB disk + ~3.1 GB RSS today, on this host, without code-level coordination. Even before the rest of the slices land, the operator can run `gc dolt cleanup --force` once be-o9z merges to free the existing leak.

#### AD-05 — bd-wrapper resilience (be-tve, P2)

| Child | Title | Routes | Blocked-by |
|-------|-------|--------|------------|
| be-3fp (P2) | bd.slow telemetry on bd-wrapper | beads/builder | — |
| be-ps0 (P2) | Bounded retry on idempotent reads in bd-wrapper | beads/builder | be-3fp |

**Why two beads.** The architect's two refinements (§5.1 slow telemetry, §5.2 bounded retry) are separable: §5.1 is observability; §5.2 is recovery behavior. Sequencing is be-3fp → be-ps0 because the retry event uses the `sanitizeBDArgs` helper introduced in §5.1.

## Hard gates carried into each build bead

Each child bead names its acceptance criteria inline. The pm cannot meaningfully add new gates beyond what the architect specified — these are the architect's criteria, lifted verbatim into the children:

- AD-01 (be-c5p): architect criteria 1–7 (panic semantics; firewall behavior; bench harness compatibility).
- AD-02 (be-a5z): architect criteria 1–5 (flag both ways; default unchanged; SQL trace zero label JOINs; conflict errors; tests).
- AD-03 (be-9u4 / be-hk6 / be-1ya): architect criteria 1–7 distributed across the three beads (stagger determinism; promote within 10; demote within 10; skip-labels argv; label-only no spurious updates; non-label change still updates; existing tests).
- AD-04 (be-o9z / be-4r4 / be-avn): architect criteria 1–6 (port discovery; dry-run accuracy; live cleanup numbers; nightly order fires).
- AD-05 (be-3fp / be-ps0): architect criteria 1–6 (slow event at 30s; no event under 30s; retry success; no retry on writes; SQL marker triggers retry on reads only; zero overhead on fast path).

## Discovered-from lineage

All 11 children are linked to their architect/designer source via `discovered-from`:

```
be-695 ← be-c5p, be-xtf
be-w3n ← be-a5z
be-4v2 ← be-9u4, be-hk6, be-1ya
be-hjj ← be-o9z, be-4r4, be-avn
be-tve ← be-3fp, be-ps0
```

Plus internal `blocks` edges:

```
be-a5z ──blocks──> be-9u4    # AD-02 → AD-03 part 1
be-o9z ──blocks──> be-4r4    # AD-04 core → formula
be-3fp ──blocks──> be-ps0    # AD-05 part 1 → part 2
```

## What's out of scope

- **Per-city sidecar reconciler** (architect doc §13). Future epic; not in any child bead.
- **Replacing reconciliation with bd's hook-event stream.** Separate effort; reconciliation stays as the catch-up watchdog.
- **IN-clause query rewrite** (architect AD-02 §"Why this over"). Once gascity stops scanning labels, the remaining IN-clause callers are minor.
- **Switching the bd-wrapper from subprocess-exec to in-process Go calls** (months-long refactor; explicitly out per architect).
- **Touching `cacheReconcileIntervalLarge`** (120 s, for >5000-bead cities). Architect: out of scope.
- **Cleanup of existing leaked databases via human run.** The operator can do this manually today (`gc dolt cleanup` once be-o9z lands; before that, `mysql ... CALL DOLT_PURGE_DROPPED_DATABASES();` per rig); it is not a build deliverable.
- **`mol-dog-dolt-purge.toml` as a separate formula.** Architect noted "only if a separate formula is preferred over inline steps"; the rewritten `mol-dog-stale-db.toml` calling `gc dolt cleanup` handles purge as a step.

## On re-escalation

If a child bead's hard gate fails — for example, the gascity reconciler's regression test (be-9u4 §5.4) reveals that `beadChanged(skipLabels=true)` still fires spurious events — the builder files a `needs-architecture` bead routed to `beads/architect` with the failure evidence. **Do not redesign the slice in-line.** The architect's decisions are the binding source of truth.

## Cross-slice constraints

- **Prefix-list convergence (AD-01 + AD-04).** be-c5p extends `testDatabasePrefixes`; be-avn extends `staleDatabasePrefixes`. Target final list (across firewall, cleanup, formula): `testdb_`, `beads_test`, `beads_pt`, `beads_vr`, `doctest_`, `doctortest_`, `benchdb_`. After both beads land, all three lists are textually identical.
- **Flag-name confirmation (AD-02 + AD-03).** be-a5z (the bd-side flag) and be-9u4 (the gascity caller) MUST agree on the flag name. The designer rename `--no-labels` → `--skip-labels` is the binding decision unless the architect overrides. Builder confirms be-a5z's status before be-9u4.

## Handoff to builder

After this plan is merged and the root beads are closed, each child is `gc sling`'d to `beads/builder` and a context-mail goes to that agent. The builder reads each child's body (architect spec + pm scope refinement + designer wireframes) and starts work in priority order: P0 first (be-o9z, be-4r4 — AD-04 operational impact), then P1 (the AD-01/02/03 chain), then P2 (be-xtf docs, be-3fp/be-ps0 wrapper resilience).

The designer's wireframes are the binding UX surface; the architect's acceptance criteria are the binding correctness surface. The pm bead is the bridge.

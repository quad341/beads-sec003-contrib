# PG-backend readiness backlog — decomposition summary

> **Parent audit:** be-21updd
> **Ship gate this backlog feeds:** be-nwwe75
> **Snapshot:** 2026-05-11
> **Sibling artifact:** [be-21updd-pg-verb-matrix.md](be-21updd-pg-verb-matrix.md)

## Critical path (blocks be-nwwe75 ship-gate lift)

```
be-nwwe75 (ship gate)
├─ blocked-by be-ry0mig   (CP-1 soak run report)        ← unblocks mayor's mem/reliability/predictability data
├─ blocked-by be-5upx19   (CP-2 benchmark run report)    ← unblocks latency table mayor wants
├─ blocked-by be-jygyks   (CP-3 lifecycle parity tests)  ← unblocks bd delete / prune / purge / rename / promote on PG
├─ blocked-by be-uvfhjk   (CP-5 migration validation rollup) ← rolls up be-6ryglx + be-njwzu1 + be-vd64cw
└─ blocked-by be-4i7ax3   (CP-4 doctor parity audit)     ← post-ship operator diagnostic readiness
```

## Tree (full decomposition)

### CP-1: Soak harness for ship-gate criteria

```
be-d52ffl (epic) — PG ship-gate soak harness: memory + reliability + predictability
├─ be-m89c7o (needs-architecture) — Design soak harness spec        → beads/architect
├─ be-6oywp4 (needs-tests, blocked-by m89c7o) — Implement harness   → beads/validator
└─ be-ry0mig (needs-tests, blocked-by 6oywp4) — Run harness, produce report → beads/validator  ★ feeds ship gate
```

### CP-2: Dolt-vs-PG benchmark comparator

```
be-cycwm6 (epic) — PG vs Dolt latency benchmark comparator
├─ be-ikvc3q (needs-architecture) — Design comparator               → beads/architect
├─ be-rzmy0a (needs-tests, blocked-by ikvc3q) — Implement comparator → beads/validator
└─ be-5upx19 (needs-tests, blocked-by rzmy0a) — Run, produce table  → beads/validator  ★ feeds ship gate
```

### CP-3: Lifecycle parity (DeleteIssues + 3 BulkIssueStore stubs)

**Status (2026-05-12):** All 4 builder beads PASS — `be-jygyks` now unblocked.
See `pg-backend-ship-gate.md` Tier 2 for cherry-pick order.

```
be-x1kg5f (epic) — PG lifecycle parity: DeleteIssues + 3 others
├─ be-ptzlki (needs-architecture) — Design semantics                → beads/architect
├─ be-kil3so ✓ PASS (DeleteIssues, gm-ll80xk)                       → builder
├─ be-efr0tm ✓ PASS (DeleteIssuesBySourceRepo, be-g8cvai)            → builder
├─ be-lj68rq ✓ PASS (UpdateIssueID, gm-u54lgr + gm-7b4d7g)           → builder
├─ be-gzztuj ✓ PASS (PromoteFromEphemeral, gm-8hk29t)                → builder
└─ be-jygyks (needs-tests, NOW UNBLOCKED) — Parity tests Dolt vs PG → beads/validator  ★ feeds ship gate
  └─ be-2bvr4u (needs-tests, non-blocking follow-up) — Poisoned-row savepoint rollback test → beads/validator
```

### CP-4: Doctor parity audit

```
be-4i7ax3 (needs-architecture) — Audit PG-doctor vs Dolt-doctor parity → beads/architect  ★ feeds ship gate
   (auditor will file builder beads for any missing-but-needed checks)
   Cross-references:
   - be-vd64cw (already open) — bd doctor --check=migration for PG
   - be-5apw1a (already open) — 5 LOW polish findings
```

### CP-5: Migration validation rollup

```
be-uvfhjk (task) — Migration validation rollup ★ feeds ship gate
├─ blocked-by be-6ryglx — one-tx assertion (P2, beads/architect — existing)
├─ blocked-by be-njwzu1 — soak corpus + interrupted-resume (P2, beads/validator — bumped from P3 by this audit, existing)
└─ blocked-by be-vd64cw — doctor --check=migration (P3, beads/architect — existing)
```

### CP-6: Standalone build bead (no design needed)

```
be-j72zr5 (ready-to-build, P2) — Replace dangling be-6fk.3 reference in PG errors.go → beads/builder
```

### Post-v1 (does NOT block ship gate)

```
be-qpea1r (epic, P3) — Post-v1 PG stubs: MergeSlot, Slot, Compaction
├─ be-yx2qjn (P3) — MergeSlot* implementation
├─ be-6fmi9x (P3) — Slot* implementation
└─ be-e35eoh (P3) — CompactionStore implementation
```

## Critical-path answer: "what's left before be-nwwe75 can lift?"

The verb matrix audit revealed that the v1 mayor-scope command path mostly works. The gaps that block ship-gate decision are:

1. **No soak data** — mayor cannot greenlift without mem/reliability/predictability evidence. Filed as CP-1.
2. **No benchmark comparator** — mayor wants latency reported even if non-binding. Filed as CP-2.
3. **Operator-facing lifecycle stubs** — `bd delete`, `bd prune`, `bd purge` all break on PG today. Filed as CP-3.
4. **Doctor parity** — post-ship operators need diagnostics on par with Dolt. Filed as CP-4.
5. **Migration validation** — already in flight via be-6ryglx, be-njwzu1, be-vd64cw; rolled up as CP-5.

The audit found that the "vanished review beads" item from the original task is **already resolved** — be-g352o7, be-8y58uy, be-65a0l2 are all findable and CLOSED with PASS verdicts captured in-bead, so no re-review is needed.

The audit also found that **POSTGRES-BACKEND.md docs** (be-eougy8) is correctly deferred behind be-qp10z7 + be-nwwe75 — no PM action needed there.

Once CP-1 through CP-5 land and their reports reach mayor, be-nwwe75 unblocks one way or the other:
- Data favors PG → rollup-ship bead cherry-picks the be-nwwe75 reviewed-clean commit list to upstream.
- Data doesn't favor PG → be-nwwe75 closes as "design exercise validated, not shipping" and the branch persists as research.

## Parallelization opportunities

- CP-1 design (be-m89c7o), CP-2 design (be-ikvc3q), CP-3 design (be-ptzlki), CP-4 audit (be-4i7ax3) can all run in parallel on the architect — they're independent design surfaces.
- CP-3 builder beads (4 stubs) can run in parallel on the builder once design lands.
- CP-5 rollup is gated by 3 architect-owned beads that may overlap with the above; coordinate with architect on capacity.
- CP-6 (be-j72zr5) is independent and can land anytime.

## Notes for future PM session

- Mayor watches the tree via `gc bd --rig beads list`. Reporting only on blockers / scope-changes (per be-21updd reporting rules).
- POSTGRES-BACKEND.md gap (be-eougy8) stays deferred until be-qp10z7 lands; re-evaluate after.
- be-5apw1a stays P3; not part of critical path.
- If architect surfaces a hidden constraint during design (CP-1 or CP-3 design), re-decompose accordingly.

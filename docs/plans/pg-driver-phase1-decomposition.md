# PG Driver Phase 1 — Full Decomposition

**Date:** 2026-05-15
**PM:** beads/pm
**Source design beads:** be-i3xud5 (closed), be-nwyp4u (closed)
**Kickoff bead:** be-utigq7 (closed)
**Mayor's Phase-1 acceptance:** be-0c0r75 (open)
**HQ epic:** gm-mdoi6b
**Ship plan:** gc-management/docs/plans/pg-driver-ship-plan.md

---

## Summary

Designer produced two comprehensive design specs from the pg-driver-ship-plan.md
and routed them to PM. This document captures the full PM decomposition into
builder/validator/architect beads.

---

## Dependency trees

### Driver interface tree (from be-i3xud5)

```
be-i3xud5 (design, CLOSED)
├─ be-7ae63g  [P1, builder]   Driver interface, DriverConfig, DriverOpener, storage.RegisterDriver
│                              → UNBLOCKED — start here
├─ be-lp2mzg  [P1, builder]   Capability type, CapabilitySet, constants, error sentinel
│                              → UNBLOCKED — start here (parallel with be-7ae63g)
├─ be-dmzgda  [P2, architect] Resolve 4 open questions (Go version, ImportRows txn, ExportRows order, registry placement)
│                              → UNBLOCKED — answer Q4 (registry) first as it blocks pkg/serde
│
├─ be-g5ppom  [P1, builder]   DoltDriver wrapping DoltStore (internal/storage/dolt/driver.go)
│   BLOCKS: be-7ae63g + be-lp2mzg
│
├─ be-aa1cqy  [P1, builder]   PGDriver stub (internal/storage/postgres/driver.go)
│   BLOCKS: be-7ae63g + be-lp2mzg
│   NOTE: must satisfy be-0c0r75 (CreateIssue uses ValidateWithCustom)
│
├─ be-azk8my  [P2, builder]   pkg/serde package (Row, RowExporter, RowImporter, WriteJSONL, ReadJSONL, Pipe)
│   BLOCKS: be-7ae63g
│
├─ be-kdh0l6  [P2, builder]   Refactor bd dolt/* commands to Capabilities().Require() dispatch
│   BLOCKS: be-lp2mzg
│
└─ be-sgda0v  [P1, validator] Mock driver test + CapabilitySet tests (bar A.3)
    BLOCKS: be-7ae63g + be-lp2mzg
```

### Archive config tree (from be-nwyp4u)

```
be-nwyp4u (design, CLOSED)
├─ be-lpfolh  [P2, builder]   archive.* config keys, archiveDefaultForBackend, bd init writes archive.*
│                              → UNBLOCKED — start immediately
│
├─ be-6lydih  [P2, builder]   export.auto backwards-compat alias + bd config set/get
│   BLOCKS: be-lpfolh
│
├─ be-tsjke3  [P2, builder]   bd doctor shows archive status
│   BLOCKS: be-lpfolh
│
└─ be-dpy9sp  [P2, validator] Archive config + alias + doctor tests (11 test cases)
    BLOCKS: be-lpfolh + be-6lydih + be-tsjke3
```

### Also unblocked (from be-utigq7 kickoff)

```
be-uo71at  [P3, builder]   bd init --help text honesty + --backend flag stub
→ UNBLOCKED, independent, can land on main immediately (no PG-backend code)

be-33hwkn  [P2, architect] Schema migration tooling decision
→ UNBLOCKED, needed before Driver.MigrateSchema() can be fully implemented

be-dmzgda  [P2, architect] Driver open questions (see above)
```

---

## Mayor's Phase-1 acceptance bead

`be-0c0r75` (open, filed by mayor) carries forward the be-rhtega fix as an
explicit Phase-1 acceptance criterion for the PGDriver implementation:

- PGDriver.CreateIssue must call `ValidateWithCustom(customStatuses, customTypes)`
- Builder of be-aa1cqy is responsible for satisfying this criterion
- Tests from be-mrhgpt (now unblocked since be-rhtega closed) cover this

---

## Sequencing principles

Per pg-driver-ship-plan.md and be-i3xud5 design:

1. Every PR adding a Driver interface method must include a PG stub for that
   method — be-g5ppom and be-aa1cqy land in the same or adjacent PRs.
2. All PRs must have beads-only repros (`go test ./...` with no gc dependency,
   no live server required).
3. PG gated behind `--experimental` until Phase 1 crosses the bar.

---

## What's NOT in Phase 1 decomposition (deferred to Phase 2)

- CI PG path (docker-compose CI): filed as a separate bead when Phase 1 nears completion
- External-eyes test (item 23): deferred to Phase 2
- bv portability (item 35): deferred to Phase 2
- Public benchmark publication: deferred to Phase 2
- Dolt future decision: deferred to Phase 2
- N-writer stress test (item 11), PG restart survival (item 13): filed when PGDriver is complete
- Multi-tenant isolation decision (item 16): needs be-33hwkn to close first

---

## Outstanding notes for future PM sessions

1. **be-33hwkn (architect: migration tooling)** — when this closes, file the
   builder bead for Driver.MigrateSchema() implementation on both Dolt and PG.

2. **be-dmzgda (architect: open questions)** — when Q4 (registry placement)
   is answered, update be-azk8my (pkg/serde) notes if needed. Other 3 questions
   can be answered after builder starts (refines rather than blocks).

3. **be-0c0r75** — closes when be-aa1cqy (PGDriver stub) satisfies the
   CreateIssue/ValidateWithCustom acceptance criterion and tests from be-mrhgpt pass.

4. **Ship gate** — the reviewed-clean queue in pg-backend-ship-gate.md tracks
   the existing PG commits on feat/be-u0zlsq-iter-interface. The new Driver
   abstraction plan may supersede the rollup approach entirely (mayor acknowledged
   this in the deferred reminder). PM will revisit after Phase 1 stabilizes.

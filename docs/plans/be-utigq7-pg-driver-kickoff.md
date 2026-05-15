# PG Driver Work Kickoff — Decomposition (be-utigq7)

**Date:** 2026-05-15
**PM:** beads/pm
**Parent bead:** be-utigq7 (closed — decomp filed)
**Plan doc:** gc-management/docs/plans/pg-driver-ship-plan.md
**HQ epic:** gm-mdoi6b

---

## Context

The mayor filed be-utigq7 to kick off the beads-rig decomposition of
`pg-driver-ship-plan.md`. That plan introduces a formal `Driver` interface
for beads, with Dolt as the reference implementation and Postgres as the
second, plus supporting infrastructure (pkg/serde, archive config, bd init
improvements).

This document captures the PM decomposition. Phase 2 (public release) is
NOT decomposed here — it is deferred until Phase 1 is demonstrably passing.

---

## What's already tracked (no new bead needed)

The following Phase 1 items from the plan are already in flight in the
beads rig or the HQ rig:

- **Doctor parity (H.29)** → be-nzo2iu (ready-to-build, filed 2026-05-15)
- **DSN/defaults config (F.19 partial)** → be-zdhe4l (architect, open)
- **BackendInfo foundation** → be-brdzcs (closed), be-p5j4lx, be-mwzf38, be-4a6vf8 (filed)
- **PG ship-gate lifecycle CPs** → tracked in HQ rig (gm-* prefix), see pg-backend-ship-gate.md

---

## Phase 1 decomposition (new beads)

### Designer track

| Bead | Title | Priority | Status |
|------|-------|----------|--------|
| be-i3xud5 | Design: PG Driver interface, capability declarations, and pkg/serde layer | P1 | open → beads/designer |
| be-nwyp4u | Design: archive.* config feature and export.auto backwards-compat alias | P2 | open → beads/designer |

**be-i3xud5** is the critical-path design item. The Driver interface must be
designed before builders can implement it. Acceptance: design doc at
`docs/designs/pg-driver-interface.md`, Go interface method signatures,
capability declarations type, pkg/serde row-iterator spec.

**be-nwyp4u** is independent of the Driver interface and can proceed in
parallel. Acceptance: design doc at `docs/designs/archive-config.md`,
config keys, driver-appropriate defaults, export.auto alias plan.

### Architect track

| Bead | Title | Priority | Status |
|------|-------|----------|--------|
| be-33hwkn | Schema migration tooling decision — golang-migrate vs per-driver ad-hoc | P2 | open → beads/architect |

This covers the open question in the plan's §'Open questions': how do future
bd schema changes ship for each driver? Decision affects the Driver interface
(does the Driver expose `Migrate()`?) and the CI setup.

### Builder track

| Bead | Title | Priority | Status |
|------|-------|----------|--------|
| be-uo71at | Build: bd init --help text honesty + --backend flag stub (item 28) | P3 | open → beads/builder |

This is a cheap Phase 1 win: remove the false "Dolt-only" claim from
`bd init --help`, add a `--backend` flag stub that rejects non-experimental
PG use. No PG-backend code — can land on main immediately.

---

## Dependency graph

```
be-utigq7 (closed — kickoff)
├─ be-i3xud5  (designer) — Driver interface + pkg/serde  [P1, critical path]
├─ be-nwyp4u  (designer) — archive config                [P2, parallel]
├─ be-33hwkn  (architect) — migration tooling decision   [P2, parallel]
└─ be-uo71at  (builder) — bd init help text              [P3, parallel]
```

After be-i3xud5 lands (designer delivers design spec), PM will decompose it
into builder beads for the Driver interface implementation.

---

## What PM will do when designer delivers

When beads/designer files `source:actual-designer` beads back to PM:

1. Decompose each design spec into concrete builder beads (one per interface
   method group or feature area)
2. Set deps: each builder bead blocked by the design bead it implements
3. Route to beads/builder and beads/validator per the bar items

---

## Phase 2 deferred

Phase 2 items (CI parity, public docs, external-eyes test, bv portability,
performance publication, Dolt future decision) are intentionally not
decomposed. Phase 2 kicks off when the mayor signals Phase 1 bar items A–G
are demonstrably passing.

---

## Open questions (for future PM sessions)

1. **be-rhtega (deferred, needs-pm):** PG CreateIssue custom-types fix — deploy
   blocked on PG rollup ship. Pending mayor greenlight. 8 days past defer date
   (2026-05-08). Mayor has not responded to PM mail (gm-39xosf, 2026-05-07).
   Re-ping mayor in next PM session.
2. **Concurrency stress test (D.11) and PG restart survival (D.13):** Not yet
   filed. These need a validator bead once the Driver interface design lands.
3. **Multi-tenant isolation decision (E.16):** Per-rig PG role vs single shared
   role — open question from plan. File an architect bead when be-33hwkn
   (migration tooling) is resolved.

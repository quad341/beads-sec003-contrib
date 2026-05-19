# Plan: docs/POSTGRES-BACKEND.md — Operator-Facing PostgreSQL Backend Reference

**Parent bead:** be-c9qj (re-anchor for vanished be-eougy8)
**Design source:** `/home/jaword/projects/gc-management/.gc/worktrees/beads/designer/be-eougy8-design.md` (603 lines, 2026-05-11)
**Target file:** `docs/POSTGRES-BACKEND.md` in the beads rig root
**Date:** 2026-05-19

## Background

The original bead be-eougy8 (operator-facing doc parity with `docs/DOLT-BACKEND.md` for the postgres backend) vanished from the database due to the known bead-vanishing phenomenon. The designer pass was completed and preserved on disk. This plan re-anchors the work under be-c9qj and decomposes it into 5 writer beads.

## Design summary

The designer fixed the information architecture for a 12-section operator-journey document. Two sections (§2 decision-criteria, §6 authentication) were drafted as ready-to-land prose. The remaining 10 sections were reduced to outlines pointing at source files.

**Section order (from design §1):**
1. Overview (feature table)
2. When to choose Postgres vs. Dolt ← drafted
3. Prerequisites
4. Quick Start
5. Connection Strings (DSN forms)
6. Authentication: how the password flows ← drafted
7. Backup and Restore
8. Migration from Dolt
9. Operational Gotchas
10. Troubleshooting
11. Configuration Reference
12. See Also

## Decomposition

Beads B1–B4 are independent and can run in parallel. B5 assembles after all four are closed.

| Bead | Sections | Blocker |
|---|---|---|
| be-c9qj.1 | §2 (pg vs Dolt) + §6 (Auth) — verbatim transcription | — |
| be-c9qj.2 | §1 (Overview) + §3 (Prerequisites) + §4 (Quick Start) | — |
| be-c9qj.3 | §5 (DSN forms) + §7 (Backup/Restore) + §11 (Config Reference) | — |
| be-c9qj.4 | §8 (Migration) + §9 (Operational Gotchas) + §10 (Troubleshooting) + §12 (See Also) | — |
| be-c9qj.5 | Assembly: stitch sections, add TOC, verify cross-links, final placement | B1+B2+B3+B4 |

## Key constraints for the builder

- Output path: `docs/POSTGRES-BACKEND.md` (NOT under `docs/tmp-postgres/`)
- Heading depth: h1 title, h2 sections, h3 subsections — monotonic, no skipping
- No emoji, no icon-only callouts
- Code blocks: triple-backtick fenced with language tag; no `$ ` prompt prefix; long lines use `\` continuation
- Tables: GFM with header rows on every table
- Cross-link text: descriptive (not "see here")
- Dolt-specific sections omitted entirely (not mentioned as absent)
- `SQLSTATE 28P01` and `BEADS_POSTGRES_PASSWORD` appear verbatim in §6 + §11 for find-in-page/search indexing

## Routing

All beads routed to `beads/builder` with label `ready-to-build`.

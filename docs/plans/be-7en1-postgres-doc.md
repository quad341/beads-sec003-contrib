# be-7en1 — docs/POSTGRES-BACKEND.md operator-facing doc parity

**Status:** decomposed; 5 child beads filed; .1 slung to builder
**Last refreshed:** 2026-05-12 by beads/pm
**Originating bead:** be-eougy8 (vanished mid-session 2026-05-11) → re-filed as be-7en1 by beads/designer

## Why this doc exists

Designer pass on `docs/POSTGRES-BACKEND.md` landed 2026-05-11 but the
originating bead `be-eougy8` vanished mid-session (see persistent
memory `bead-vanishing-phenomenon-2026-05-11-beads-created`). Designer
re-filed as `be-7en1` carrying the full design (~28KB, 604 lines) in
the bead description.

This plan doc is the PM-side mirror in case be-7en1 also vanishes:
canonical design lives in three places (bd, /tmp, designer worktree)
plus this plan doc summarizes the decomposition.

## Decomposition

PM decomposed per designer's recommended 5-bead split. Each child
bead has acceptance criteria targeting independent sections; bead .1
lays the file skeleton + verbatim drafted prose (§2, §6); .2/.3/.4
fill placeholder blocks in parallel; .5 stitches.

| Child | Bead ID | Sections | Blocked by |
|---|---|---|---|
| be-7en1.1 | **be-5954** | Skeleton + §2 Choose pg vs Dolt (verbatim) + §6 Authentication (verbatim) | — |
| be-7en1.2 | **be-9e34** | §1 Overview + §3 Prerequisites + §4 Quick Start | be-5954 |
| be-7en1.3 | **be-xush** | §5 DSN forms + §7 Backup + §11 Config Reference | be-5954 |
| be-7en1.4 | **be-jhho** | §8 Migration + §9 Gotchas + §10 Troubleshooting + §12 See Also | be-5954 |
| be-7en1.5 | **be-pq52** | TOC + stitch + cross-link verification + conventions check | be-9e34, be-xush, be-jhho |

All children:
- `--type task -p 2`
- Labels: `ready-to-build,source:actual-pm,docs,postgres`
- Metadata: `gc.routed_to=beads/builder`
- parent-child to be-7en1

## Placeholder protocol

Bead .1 lays placeholder blocks with this exact marker format:

```
<!-- BEGIN-§N -->
<!-- TODO: filled in by be-7en1.<bead-id> -->
<!-- END-§N -->
```

Siblings .2/.3/.4 replace ONLY their assigned blocks via Edit tool
(unique anchors prevent merge conflicts when multiple builders run
in parallel). Bead .5 strips the markers in the final pass.

## Bead-vanish fallback paths

If `bd show be-7en1` fails (the parent vanishes), builders can fall
back to the design at any of:

1. `/tmp/designer-be-eougy8/design.md` (28KB, volatile)
2. `/home/jaword/projects/gc-management/.gc/worktrees/beads/designer/be-eougy8-design.md` (28KB, designer worktree — may not be visible from builder worktree)
3. This plan doc summarizes the structure; the children's acceptance criteria reference section-by-section IA but the verbatim prose (§2, §6) requires the design file.

## Source documents

- Designer mail: gm-aq6b2r (vanish notice), gm-qoj1g2 (re-filed as be-7en1)
- PM seat mail to builder: gm-* (after this commit)
- Originating bead: be-eougy8 (vanished) → be-7en1 (re-filed)
- Parity reference: `docs/DOLT-BACKEND.md` (517 lines)
- Existing pg context: `docs/AUDIT_TRAIL_POSTGRES.md`, `docs/tmp-postgres/PROJECT_MANIFEST.md`

## Outstanding decisions

None at decomposition time. If builders surface issues during
implementation:
- Stale source-code references (function moved, line numbers drifted)
  → builders should update doc to match current main and record what
  changed in their bead notes; PM updates this doc.
- Missing external doc cross-links (e.g., `docs/AUDIT_TRAIL_POSTGRES.md`
  absent) → bead .5 surfaces in its notes; PM beads the missing doc
  separately.
- CLI bugs discovered during flag-verification → builders file
  discovered-from beads; PM routes to architect if guardrails are
  needed.

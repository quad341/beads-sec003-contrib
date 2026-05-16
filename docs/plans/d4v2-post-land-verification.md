# Plan: D4v2 post-land verification

**Date:** 2026-04-30
**Author:** beads/pm (acting on be-sz8a designer audit handoff)
**Audit predecessor:** be-sz8a (designer)
**Architect decision:** be-08pl
**ADR:** be-eei
**Build artifact:** be-s54 / cfd38cda (migration 0033)

## Why this plan exists

Migration 0033 (composite `(status, updated_at)` + standalone
`defer_until` indexes on `issues`) lands on cfd38cda under the
architect's model-based-waiver supplement to NFR-D4-3 (be-08pl §1).
The waiver is conditional, not a relaxation: NFR-D4-1 / NFR-D4-2 (the
10% Create / Update regression ceilings at 10K) still bind, and the
architect has mandated post-land empirical verification (be-08pl §5)
within a hard deadline.

This plan tracks pm's three deliverables from the designer audit
(be-sz8a §C.2):

1. File the verification bead.
2. Decide be-s9d's disposition (retarget vs. close-as-superseded).
3. Hand the new bead ID to the integration owner for the PR-body
   placeholder substitution.

## What was done

### Verification bead — filed as be-6e8s

Created with the architect's quote-ready spec from be-sz8a §C.1
verbatim:

- **Title:** *Post-land verification: D4v2 write-bench at 10K
  (be-08pl §5)*
- **Priority:** P1 (gates the waiver's retroactive justification).
- **Routing:** `gc.routed_to=beads/builder`, label
  `ready-to-build`.
- **Labels:** `perf`, `schema`, `verification`,
  `source:actual-pm`, `ready-to-build`.
- **Dependencies:**
  - `discovered-from` be-08pl (architect decision).
  - `discovered-from` be-sz8a (designer audit).
  - `related` be-eei (D4v2 ADR).
- **Acceptance criteria:** measurement at `-count=5` on host-resident
  dolt sql-server (`/var/tmp/be-s9d/dolt/`), `benchstat -geomean`
  comparison vs. pre-D4v2 baseline, both Create and Update
  regressions ≤ 10% at α=0.05, dolt `go.mod` drift noted, panic
  reproduction status noted.
- **Rollback condition:** any > 10% regression at α=0.05 on Create
  or Update → migration 0034 reverting via
  `0033_add_date_indexes.down.sql`, architect re-decides D4 from
  scratch.
- **Deadline:** earlier of (a) 30 days post-integration land, or
  (b) be-8xg9 (upstream dolt `journalWriter.flush` panic) is
  resolved. If neither happens, file a fresh re-escalation bead —
  do not silently extend.

### be-s9d — retargeted (architect §6.3 path)

Designer (be-sz8a §C.2) named two architect-blessed options:
retarget vs. close-as-superseded. Architect §6.3 directs retarget
explicitly:

> The 10K write-bench part becomes superseded by this decision …;
> it is retargeted to track the post-land verification of §5.
> Append a note retargeting the bead. Add depends-on link to the
> new verification bead. Re-route to builder.

Followed verbatim:

- Appended a retargeting note to be-s9d explaining the scope shift.
- Added `bd dep add be-s9d be-6e8s --type blocks` (be-6e8s blocks
  be-s9d → be-s9d sits blocked until verification closes).
- Routing stays at `gc.routed_to=beads/builder`.
- be-s9d's other scope (testmain external-port ordering bug,
  independently addressed on `tests/be-s9d-testmain-external-port`
  / commit 27a4f1f8) is preserved as historical context in the
  bead body.

**Why retarget instead of close:** keeps the bead as a tracker so
the testmain branch and the bench verification can co-resolve. The
architect's directive was unambiguous; designer's
"both-blessed" framing was permissive but not equivocal — the
architect's explicit direction is the binding word.

### Integration PR placeholder

The integration PR for cfd38cda must satisfy be-08pl §7.2
supplemented guardrail 6 (architectural-provenance block; see
be-sz8a §B.5). The block contains a placeholder:

```
5. ✓ Post-land verification follow-up filed (see be-<verification-bead-id>).
```

**Substitution:** `<verification-bead-id>` → `be-6e8s`.

This substitution is the integration owner's responsibility (likely
builder, after reviewer's re-pass on be-nx7). Pm's handoff:
mailing builder + reviewer with the bead ID.

## What was NOT done (per audit recommendations)

Designer (be-sz8a) explicitly recommended **no changes** to:

- `bd migrate` runner output (no "provisional" diagnostic added).
- be-8ja runner wording (60s ceiling stays).
- CHANGELOG.md user-facing copy at cfd38cda (already accurate).

Pm filed no beads for these — they're closed in the audit.

## Linked beads

| Bead    | Role                                          | Status         |
|---------|-----------------------------------------------|----------------|
| be-eei  | D4v2 ADR (architect)                          | closed         |
| be-08pl | NFR-D4-3 supplement (architect, model waiver) | closed         |
| be-s54  | D4v2 build (cfd38cda)                         | (build artifact) |
| be-nx7  | D4v2 reviewer pass (re-route to reviewer)     | open           |
| be-yhs  | D4v2 design audit (designer, pre-waiver)      | (predecessor)  |
| be-sz8a | D4v2 designer audit (waiver-aware)            | closing now    |
| be-s9d  | 10K write-bench + testmain bug (sister)       | retargeted     |
| be-6e8s | **Post-land verification (this plan's output)** | **open**       |
| be-8xg9 | Upstream dolt panic (deep-investigator)       | open           |

## Handoff

- `gc sling beads/builder be-6e8s` — wakes builder.
- Mail builder: bead ID + integration-PR placeholder substitution.
- Mail reviewer (beads/reviewer): pointer to be-6e8s for the
  re-pass on be-nx7 (so reviewer can confirm the verification bead
  exists when validating §7.2 provenance block).
- Close be-sz8a.
- Mail mayor: brief decision summary (be-s9d retarget was the
  judgment call; surfacing it in case the user wants to redirect).

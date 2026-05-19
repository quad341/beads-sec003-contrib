# Plan: `bd init` Fork-Detection UX Implementation

**Goal bead:** be-0ccf34 (design), be-c7696f (implementation)  
**Architecture:** bd-umbf Child 1 + Child 4  
**Date:** 2026-05-17  
**Author:** beads/pm

---

## Summary

The designer completed the UX spec for the auto-fork-detection path in `bd init`. This plan
converts that design into two implementation beads: a builder bead for the init.go changes and
a validator bead for test coverage.

---

## Context

The bd-umbf architecture (contributor namespace isolation) requires `bd init` to auto-detect
fork repos (via presence of an `upstream` git remote) and silently configure contributor
routing — without requiring the user to pass `--contributor`. The designer has resolved all
UX decisions in `be-0ccf34-design.md`:

- Auto-configure without prompting (FR-02 zero-friction)
- Opt-out via `--role=maintainer` flag (already exists)
- Three output variants: happy path, opt-out, re-init/already-configured
- CI/non-interactive: silent routing, no fork block

---

## Child beads

### be-c7696f — Auto-configure contributor routing in bd init (builder)

**Status:** open → `ready-to-build` (design complete)  
**Target:** beads/builder

**Scope:** Modify `cmd/bd/init.go`:
1. After Dolt DB committed, before auto-export prompt (~line 1201 `promptForkExclude`), call fork-detection logic
2. `git remote get-url upstream` → isFork detection
3. If isFork and routing not configured and not `--role=maintainer`: auto-configure contributor routing, print happy-path block
4. If isFork and `--role=maintainer`: print opt-out block, skip routing
5. If isFork and routing already configured: print already-configured block, skip
6. If CI/non-interactive: configure silently, no output block

**Exact output format:** See `be-0ccf34-design.md` terminal mockups (happy path, opt-out, re-init, CI sections).

**UI primitives:** `ui.RenderAccent("▶")`, `ui.RenderPass("✓")`, `ui.RenderWarn("⚠")` — no new components.

---

### [validator bead] — Tests: bd init fork-detection auto-routing

**Status:** open (`needs-tests`)  
**Target:** beads/validator  
**Blocked by:** be-c7696f

**4 test cases:**

| Test name | Setup | Assert |
|-----------|-------|--------|
| `TestBdInit_ForkAutoContributor` | upstream remote present, no existing routing | routing.mode=auto set, routing.contributor=~/.beads-planning, beads.role=contributor, ▶ block in stdout |
| `TestBdInit_ForkAutoContributor_Idempotent` | run bd init twice | second run: ⚠ already-configured block, no config changes |
| `TestBdInit_ForkAutoContributor_MaintainerFlag` | upstream remote + `--role=maintainer` | routing NOT configured, ⚠ skipped block in stdout |
| `TestBdInit_ForkAutoContributor_NonInteractive` | `CI=true` env | routing configured, no fork block in stdout |

Use existing init test helpers; embed fake git repo with upstream remote configured.

---

## Dependency graph

```
be-0ccf34 (design) ──✓ closed
    └── be-c7696f (builder: ready-to-build) ──✓ closed (implemented pre-design spec)
            └── be-7daa14 (builder: UX alignment to designer spec) ──✓ closed (PR #4028, builder)
                    └── be-de99a6 (validator: needs-tests) ← NOW READY
```

---

## Status (2026-05-19 — FINAL)

| Bead | Role | Status |
|------|------|--------|
| be-0ccf34 | Designer: UX spec | ✓ closed |
| be-c7696f | Builder: initial impl | ✓ closed (pre-design) |
| be-7daa14 | Builder: UX alignment | ✓ closed (commit 9ed33b68d, PR #4028) |
| be-de99a6 | Validator: 4 test cases | open — slung to validator 2026-05-19 |
| be-bxeb | Builder-filed validator duplicate | ✓ closed (covered by be-de99a6) |
| be-f9a104 | bd-umbf Children 1-3 tests | open — routed to validator 2026-05-19 |
| be-dsgn01 | PM: decompose design | ✓ closed |

### bd-umbf overall (PR #4023) — open blockers

Tracked in review bead **be-72b53f** (routed to builder, `ready-to-build`):

| Blocker | Description | Status |
|---------|-------------|--------|
| B1 | Doc freshness CI: bd migrate-personal missing from docs/CLI_REFERENCE.md | tracked in be-72b53f |
| B4 | migrate-personal: no DB-level transaction on delete path | tracked in be-72b53f |

- **B2** (ubuntu test suite): ✓ now green
- **Output format** (N1 from review): ✓ fixed by 9ed33b68d and PR #4028

**Validator note:** be-de99a6 and be-f9a104 slung to validator 2026-05-19. Tests target
commit 9ed33b68d on feat/be-jewoem-be-u2mw2x-reference-aware-prune.

---

## What this plan does NOT cover

- `bd export --exclude-owner` — tracked in be-9d2b67
- `bd migrate-personal` — tracked in be-3c5a0f
- `bd preflight` personal-issues check — deferred to bd-lfak epic

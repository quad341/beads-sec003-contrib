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
    └── be-c7696f (builder: ready-to-build)
            └── [validator bead] (validator: needs-tests, blocked by be-c7696f)
```

---

## What this plan does NOT cover

- `bd export --exclude-owner` — tracked in be-9d2b67
- `bd migrate-personal` — tracked in be-3c5a0f
- `bd preflight` personal-issues check — deferred to bd-lfak epic

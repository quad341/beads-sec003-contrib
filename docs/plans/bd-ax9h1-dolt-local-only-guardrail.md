# Plan: bd local-only Dolt guardrail (be-ax9h1)

**PM:** beads/pm  
**Date:** 2026-05-22  
**Status:** Implementation ready — 2 builder beads created

---

## Problem

Recurring city-health incident: agents "fix" a `bd dolt push` non-fast-forward error
by running `bd dolt remote add origin <git-origin-url>` then calling `DOLT_PULL`.
Result: Dolt sql-server CPU runaway (~180%) and private bead data pushed to public git.
Memory-based workaround (`bd remember`) fails to survive agent restarts.

---

## Architecture reference

See be-ax9h1 (closed) for full architecture doc, requirements, and guardrails.

---

## Design references

- **be-0jyq7** — UX design for `dolt.local-only` enforcement messages (push/pull no-op, remote-add refusal, help text)
- **be-fxwbm** — UX design for git-origin collision guard messages and `bd doctor` warning check

---

## PM decisions

| Question | Decision |
|----------|----------|
| JSON mode for push/pull no-op | YES: emit `{"status":"disabled","reason":"dolt.local-only=true"}` to stdout, exit 0 |
| `bd dolt status` local-only indicator | YES: single line "Remote sync: disabled (dolt.local-only=true)" |
| Multiple matching remotes in doctor | Combined warning naming ALL matching remotes; one Fix line per remote name |
| `doctor.CategoryDolt` constant | Builder creates it if absent; do not use CategoryGit as stopgap |
| `--allow-git-origin` in help | YES: documented with note "Use only for intentional git-backed Dolt storage" |

---

## Implementation beads

### be-3v2ou — dolt.local-only enforcement (FR-01 to FR-05)

**Routes to:** builder  
**Scope:** cmd/bd/dolt.go, cmd/bd/dolt_autopush.go, internal/config/yaml_config.go

Key deliverables:
- `isDoltLocalOnly()` helper
- `bd dolt push` / `bd dolt pull` no-op guards (exit 0, stdout message)
- `bd dolt remote add` local-only refusal (exit 1, stderr, before `getStore()`)
- `adoptGitOriginRemoteForPush()` and `maybeAutoPush()` short-circuits
- `YamlOnlyKeys["dolt.local-only"]` entry
- Help text update, `bd dolt status` indicator, JSON mode support
- Integration tests

### be-7eu1d — git-origin collision guard (FR-06 to FR-09)

**Routes to:** builder  
**Blocked by:** be-3v2ou (must merge first; shares `isDoltLocalOnly()` helper)  
**Scope:** cmd/bd/dolt.go, cmd/bd/doctor.go (or doctor_dolt.go)

Key deliverables:
- `gitOriginGetURL()`, `normalizeRemoteURL()`, `doltRemoteMatchesGitOrigin()` helpers
- `bd dolt remote add` git-origin refusal (exit 1, after local-only check)
- `--allow-git-origin` flag on `doltRemoteAddCmd.init()`
- Override warning (stderr, exit 0)
- `checkDoltRemoteMatchesGitOrigin()` doctor check (statusWarning, CategoryDolt)
- Integration tests

---

## Dependency graph

```
be-ax9h1 (arch, closed)
  ├── be-0jyq7 (design: local-only UX, closed)
  │     └── be-3v2ou (impl: local-only enforcement) ← builder starts here
  └── be-fxwbm (design: git-origin guard UX, closed)
        └── be-7eu1d (impl: git-origin guard) ← blocked until be-3v2ou merges
```

---

## Exit criteria

Both beads merge to main with green CI. `bd dolt push` on a local-only project exits 0
with a clear message. `bd dolt remote add <git-origin-url>` exits 1 with guidance.
`bd doctor` shows a warning when a Dolt remote matches the git origin. The memory-based
workaround (`bd remember dolt-local-only`) can be retired after these merge.

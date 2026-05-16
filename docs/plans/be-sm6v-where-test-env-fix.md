# Plan — `cmd/bd` where-test env opt-out fix (be-sm6v)

**PM:** beads/pm · **Date:** 2026-05-12
**Parent architecture:** be-hple (architect, CLOSED) · **Design:** be-sm6v notes (designer)
**Related:** be-lspm (P3 follow-up: `initConfigForTest` hardening — separate decision)

## Goal

`TestWhereCommand_ReadsPrefixFromEmbeddedStore`
(`cmd/bd/where_cgo_test.go:18`) passes under
`./scripts/test.sh -run TestWhereCommand_ReadsPrefixFromEmbeddedStore
./cmd/bd/`. The cascading
`TestResolveWhereBeadsDir_UsesInitializedDBPath` failure resolves with
the same change. No production-code edits.

## Context

The repo dogfoods bd: `<repo>/.beads/config.yaml` sets
`issue-prefix: be`. viper picks it up during the test, the YAML branch
in `where.go:77` short-circuits, and the store-read fallback that the
test is actually asserting against is never exercised.

`BEADS_TEST_IGNORE_REPO_CONFIG=1` is the established opt-out (14
existing test files use it). The failing test simply omits it.

Architect (be-hple) confirmed this is a **test bug, not a production
bug** — the YAML-wins precedence is the documented design and is
replicated across 7+ other consumers. The sister test
`TestWhereCommand_UsesConfigPrefixFromSelectedDB` (where_test.go:326)
explicitly locks it in.

## PM judgements (designer findings)

The designer ran the architect's requested sister-test audit. Both
findings confirmed and approved:

### (a) Sister test is NOT passing by accident — APPROVED

`TestWhereCommand_UsesConfigPrefixFromSelectedDB` writes its own
`<tempRepo>/.beads/config.yaml` with `issue-prefix: yamlprefix` and
relies on `BEADS_DIR` priority loading to make that file win over the
repo's `.beads/config.yaml`. It validates yaml-wins semantics for the
right reason. **Do not touch `cmd/bd/where_test.go:326`.**

### (b) Architect Option A is the right scope — APPROVED

Single-line `t.Setenv("BEADS_TEST_IGNORE_REPO_CONFIG", "1")` before
`initConfigForTest(t)`. Options B/C/D from be-hple remain deferred /
rejected. Option B is captured separately as be-lspm (P3).

## Decomposition decision: re-route be-sm6v in place

This is a 1-line edit in one file with a single test command for
verification. Spawning a child builder bead just to copy the same
description over would be ceremony. PM re-labels be-sm6v from
`needs-pm` → `ready-to-build`, flips `gc.routed_to` to
`beads/builder`, and clears the assignee so the builder controller
picks it up.

The bead body already carries the full implementation hint, constraint
list, and verify command from the architect; the designer's audit is
in the notes. The builder has everything it needs without redirection.

## Out-of-scope for this bead

- `initConfigForTest` hardening — tracked by **be-lspm** (P3).
- The unrelated `internal/storage/issueops/search_summary.go` /
  `count.go` compile failures the designer hit while attempting a live
  sister-test check. If the tree is still broken at builder start
  time, builder should `git pull` first; if still broken after pull,
  builder files a `needs-architecture` bead and we route from there.

## Verify

```bash
./scripts/test.sh -run \
  'TestWhereCommand_ReadsPrefixFromEmbeddedStore|TestResolveWhereBeadsDir_UsesInitializedDBPath|TestWhereCommand_UsesConfigPrefixFromSelectedDB' \
  ./cmd/bd/
```

All three pass; nothing else in `./cmd/bd/` regresses.

## Handoff

Re-routed to **beads/builder** with `ready-to-build`. Slung + mailed.

# Plan — `initConfigForTest` auto-opt-out of repo config (be-3lch)

**PM:** beads/pm · **Date:** 2026-05-12
**Parent architecture:** be-lspm (architect, CLOSED) · **Design:** be-3lch notes (designer)
**Sequenced after:** be-sm6v (P2 — explicit fix) — must land first

## Goal

`cmd/bd/test_helpers_test.go` and `cmd/bd/test_helpers_pure_test.go`
both auto-set `BEADS_TEST_IGNORE_REPO_CONFIG=1` inside
`initConfigForTest`, so every test that uses the helper gets a clean
viper state without each caller having to remember the opt-out.

## Context

After be-sm6v fixes the immediate failing test, the underlying
foot-gun still lives: any future test calling `initConfigForTest`
inherits viper values from the beads repo's own `.beads/config.yaml`
(`issue-prefix: be`, plus `dolt.auto-start`, `gc.*`, `export.auto`,
`no-push`). 14 test files already work around it with explicit
`t.Setenv`. This bead makes the safe default the helper's
responsibility.

## PM judgements (designer findings)

### (a) Architect's "Risk" question resolved — APPROVED

Architect's bead body left an open uncertainty: *"the sister test
[`TestWhereCommand_UsesConfigPrefixFromSelectedDB`] must currently
rely on something else — perhaps a re-load via
`prepareSelectedNoDBContext`, or it's silently broken."* The
designer traced both viper-loading paths and confirmed: the sister
test passes for the **right** reason (BEADS_DIR-priority block
loads the test's temp `config.yaml` during the second `Initialize()`
triggered by `prepareSelectedNoDBContext`). It will continue to
pass after be-3lch — the env var only suppresses the cwd-walk
project-config path, not the BEADS_DIR-priority path.

The audit gate in acceptance criterion #4 stays as written —
cheap insurance — but no surprises expected.

### (b) Sequencing: must land after be-sm6v — APPROVED, ENFORCED

The designer's sequencing note is load-bearing:

1. be-sm6v (P2, explicit fix) lands first — preserves the bisect
   signal for the original be-hple failure.
2. be-3lch (P3, helper hardening) lands second. After it lands,
   be-sm6v's explicit `t.Setenv` at `where_cgo_test.go:19` becomes
   redundant. Removal is a separate hygiene PR; **do not bundle**.

PM encodes this with `bd dep add be-3lch be-sm6v --type blocks` so
the builder queue won't surface be-3lch until be-sm6v closes.

## Decomposition decision: re-route be-3lch in place

Same shape as be-sm6v — 1-line edit in 2 lockstep files, plus a doc
comment update. Bead body has the full implementation hint and verify
command. Spawning a child bead would be ceremony. PM re-labels
be-3lch from `needs-pm` → `ready-to-build`, sets
`gc.routed_to=beads/builder`, clears assignee, and adds the
`blocks` edge.

## Out-of-scope for this bead

- Cleanup of the 14 redundant explicit `t.Setenv` calls — separate
  hygiene PR after be-3lch lands.
- Removal of the explicit `t.Setenv` added by be-sm6v in
  `where_cgo_test.go:19` — also part of that hygiene PR.
- Any test fixes that may surface when the dogfood masking goes away
  — architect criterion #4 requires capturing those in NEW beads,
  not coupling fixes here.

## Verify

```bash
./scripts/test.sh ./cmd/bd/
./scripts/test.sh -run 'TestWhereCommand|TestResolveWhereBeadsDir|TestRouting' ./cmd/bd/
```

`cmd/bd/` passes end-to-end. If
`TestWhereCommand_UsesConfigPrefixFromSelectedDB` (or any other test)
fails, file a NEW bead — do not silence or in-bead fix.

## Handoff

Re-routed to **beads/builder** with `ready-to-build`. Blocked-by
be-sm6v; will surface naturally when be-sm6v closes. Builder mailed
with explicit sequencing context.

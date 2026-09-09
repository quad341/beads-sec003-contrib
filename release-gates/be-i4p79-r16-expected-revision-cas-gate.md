# Release gate: R16 per-record CAS (expectedRevision)

- Deploy bead: be-i4p79
- Build bead: be-x5jqd.3
- Review bead: be-9bzeh (verdict: pass)
- Branch: builder/be-x5jqd.3
- Reviewed commit (BEFORE_SHA): 03f81b9fd222e565414619d1c2f3b4c4bac3b5a3
- Post-self-rebase commit (AFTER_SHA): f92e02458bbce5ac46e7de1eec85b0e9214fe59a
- BASE_REF: main @ a690b0a8c
- DEPLOY_MODE: remote (origin points at github.com, fetch-only; push target headfork)

## Criteria

1. **Reviewer PASS verdict present** — PASS. be-9bzeh `verdict: pass`.

2. **Acceptance criteria met** — PASS. Accepted be-9bzeh's own twice-independently-verified
   resolution of the one previously-uncovered criterion (acceptance criterion 5, the
   cross-rig `ga-9t7vpl` push-blocker): fix commit `c5b52961d31` (PR #6116) landed on
   gc-management's own main 2026-09-07T18:47:56Z, over 30h before be-x5jqd.3 started
   (2026-09-09T01:44:31Z). That SHA belongs to the gc-management repo, not this one, and
   is not independently re-checkable from this worktree; not re-verified here for that
   reason, deferring to the reviewer's already-thorough evidence.

3. **Tests pass** — **FAIL.**

   `TestEpochContract` (all 3 storage legs: dolt, embeddeddolt, uow) is diff-owned:
   `internal/storage/{dolt,embeddeddolt,uow}/retention_epoch_contract_test.go` are
   wholly new files (`git diff --diff-filter=A`, 100% insertions, 341 lines across the
   3 legs) added by commits `fd5ea2978`/`b3483786f` in this diff, and `TestEpochContract`
   itself is a newly-added function (confirmed via `git diff ... | grep '^+.*func
   TestEpochContract'` → `+func TestEpochContract(t *testing.T) {`), not pre-existing
   code in a merely-touched file.

   Independently re-verified fresh this gate round (not relying on the reviewer's
   recorded evidence, per this formula's requirement to be the independent
   re-verification): ran `./scripts/test.sh -v --run TestEpochContract` per leg on the
   current (post-self-rebase) branch tip, with
   `BEADS_TEST_ENV_RUN_DOLT=1 BEADS_TEST_EMBEDDED_DOLT=1` and a live podman-backed
   `dolthub/dolt-sql-server:2.2.0` container. All 3 legs FAIL identically:

   ```
   --- FAIL: TestEpochContract (0.00s)
       --- FAIL: TestEpochContract/AnEpochBumpIsTriggeredOnlyByRestoreReinitOrSchemeChange (0.00s)
       --- FAIL: TestEpochContract/EpochBumpVoidsOnlyAddressesOfVersionsNoLongerServed (0.00s)
   ```
   with message `R20 epoch-transition enforcement is not implemented yet on this leg
   (be-x5jqd.4)` — matching the reviewer's own prior finding exactly.

   Rule 3a clause (i) is an absolute bar regardless of evidence quality: *"If the diff
   added or modified that test file, it is a hard FAIL — no attribution, no
   exception."* That applies here without ambiguity — this is not the edge case of a
   touched-but-unchanged function in an otherwise pre-existing file; the file and the
   function are both new.

   The reviewer's own `attribution_evidence` (`out_of_diff` claim) for this failure is
   independently checked here and does not hold: `git diff --name-only
   2bb1e20de0f0072d7600656ea3cb7f606929dcc6...03f81b9fd222e565414619d1c2f3b4c4bac3b5a3`
   (the reviewer's own cited base/commit range, 98 files — matching their own recorded
   evidence line) includes all 3 `retention_epoch_contract_test.go` paths. "Zero overlap"
   is incorrect for this specific failure; the reviewer's check appears to have run
   against a narrower file list (likely the 16/18-file subset used for style/security
   review) rather than the full diff their own text names.

   No genuine mayor/operator-granted `waiver_ref` exists for this failure:
   - be-i4p79's own metadata carries no `waiver_ref` field at all.
   - be-9bzeh's `waiver_ref` note ("No new waiver needed beyond the one already
     established earlier in this review...") reads as the reviewer's own internal
     determination, not an external mayor/operator grant — which the rule explicitly
     disallows ("you may not grant your own").
   - `bd search "waiver"` surfaces only be-mf09o, the general architecture design for
     the attribution/waiver mechanism itself (closed 2026-08-19) — not a specific grant
     for this bead or this test.
   - `gc mail inbox` is empty; no mayor ruling found on this specific failure.

   Per the rule: *"With no waiver: record FAIL and escalate rather than substituting a
   conformance audit or self-certifying."*

   - `diff_tests_executed`: `TestEpochContract` (dolt): FAIL (both subtests); `TestEpochContract`
     (embeddeddolt): FAIL (both subtests); `TestEpochContract` (uow): FAIL (both
     subtests). Other diff-owned tests not independently re-run this round — the
     diff-owned FAIL above is already dispositive for the verdict; reviewer's own record
     shows 14/14 PASS on the remaining diff-owned top-level tests (not independently
     re-verified here).
   - `test_cmd_scope`: focused (`--run TestEpochContract` per leg) — sufficient to
     establish the dispositive FAIL; full-suite not required to reach a FAIL verdict
     once a diff-owned test-FAIL with no waiver is already established.
   - `waiver_ref`: none.
   - `policy_lane` (3b): `make ci-pr-policy` → exit 1, but the sole failure is the
     pre-documented `.githooks/commit-msg` BEADS-INTEGRATION-marker false positive
     (confirmed via `git check-ignore -v`: excluded via `.git/info/exclude`, untracked,
     a session-local shim, not a real repo file — unrelated to this diff, known to
     recur every gate round). No other policy/lint failures.
   - `ci_lane_run` (3c): n/a — no CI-config files in the diff (`git diff --name-only
     ... | grep -E '\.github/workflows|\.ya?ml$'` → empty).

4. **No high-severity review findings open** — PASS. be-9bzeh's style/security findings
   are all non-blocking; `spec_findings` empty.

5. **Feature branch clean** — PASS. `git status --short` empty on `builder/be-x5jqd.3`.

6. **Branch diverges cleanly from BASE_REF** — PASS, via bounded self-rebase
   (`BEFORE_SHA=03f81b9fd2` → `AFTER_SHA=f92e02458b`, `rc=0`, zero conflicts, pushed to
   `headfork`).

## Overall verdict: FAIL

Blocking: criterion 3 (diff-owned test `TestEpochContract`, all 3 legs, FAIL, no
attribution possible under rule 3a(i), no mayor/operator waiver on file).

Routed back to builder with `ready-to-build` label; escalated to mayor per the
no-waiver escalation template. The separate ancestry-scope concern (16 stray commits
citing GitHub PR/issue numbers rather than bead ids, expected to independently fail
`assert_deploy_ancestry_scope` at push-and-pr) was not reached — this gate fails at
evaluate-gate, before push-and-pr, and is not reconciled with that concern here.

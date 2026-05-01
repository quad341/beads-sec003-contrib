# Release gate — be-jdoy (be-xtf docs/TESTING.md test-isolation section)

**Date:** 2026-04-30
**Deployer:** beads/deployer (deployer-1)
**Bead (review):** be-jdoy — Review: be-xtf docs/TESTING.md test-isolation section
**Feature bead:** be-xtf (closed; source bead)
**Reviewed commit:** `fd11920c` on `be-xtf-readme`
**Base:** `origin/main` @ `8694c535` ("doctor: detect AGENTS.md / CLAUDE.md user-authored divergence (#3600)")

## Verdict: FAIL

## Reason

**Criterion #2 (Acceptance criteria met) — FAIL.** The shipped docs
describe symbols that do not exist on `origin/main` and have no
`needs-deploy` review bead in the deploy queue.

The `docs/TESTING.md` "Test Isolation" section added by `fd11920c`
references three things that AD-01 (be-c5p, commit `47dcc380`) introduces
but has not yet shipped to `origin/main`:

- `BEADS_TEST_SERVER` env var — not defined on `origin/main`.
  `git grep BEADS_TEST_SERVER origin/main` returns nothing.
- `BEADS_PRODUCTION_PORT` env var — not defined on `origin/main`.
- `isProductionPort` function — not defined on `origin/main`. The
  docs say "see `isProductionPort` and the database-name firewall in
  `New`"; only the latter (`isTestDatabaseName`, added by upstream
  `408333ff`) is on `origin/main`.

The reviewer's hand-off note explicitly anticipated this:

> Per builder note: branch be-xtf-readme is off ee67d5f9 (be-vzu-rebase-fix HEAD).
> The docs travel with be-c5p (47dcc380) which they describe. Deploy sequencing:
> deploy be-vzu-rebase-fix first (be-l9q), then be-xtf-readme on top, OR
> fast-forward be-xtf-readme onto whatever ships be-c5p.

Neither precondition is met:

- `be-vzu-rebase-fix` has not shipped. Its only deployed slice is be-a5z
  (`bd list --skip-labels`) via PR #3562, which does NOT include be-c5p.
- be-c5p has no `needs-deploy` review bead in the deployer queue. It
  cannot be shipped until it goes through review.

Shipping be-jdoy alone would publish documentation pointing at
non-existent env vars and functions. End users following the docs would
set `BEADS_TEST_SERVER=1` and find that beads doesn't recognise it.

## Criteria walk

| # | Criterion | Result | Notes |
|---|-----------|--------|-------|
| 1 | Review PASS present | PASS | reviewer-gm-po6pox3 PASS verdict in be-jdoy notes (0 blockers, 0 high/med, 1 advisory low). |
| 2 | Acceptance criteria met | **FAIL** | Docs reference unshipped symbols (see "Reason" above). The literal AC checklist (3 env vars in table, troubleshooting paragraph, store.go cross-ref, markdown renders) is met against the bead spec, but the spec presupposed be-c5p shipping first — which it has not. |
| 3 | Tests pass | n/a | Docs-only change; no tests added or affected. |
| 4 | No HIGH-severity review findings open | PASS | 0 HIGH/MED findings; 1 LOW (BEADS_DOLT_SERVER_PORT vs BEADS_DOLT_PORT discoverability) noted as a follow-up, not a blocker. |
| 5 | Final branch is clean | n/a | (No final branch cut — gate FAILed before push.) |
| 6 | Branch diverges cleanly from main | PASS-on-mechanics | Cherry-pick of `fd11920c` onto `origin/main@8694c535` applies textually with no conflict. The block here is semantic (forward-references unshipped symbols), not mechanical. |

## What needs to happen

One of:

1. **Ship be-c5p first.** Get a review bead written for `47dcc380`
   (be-c5p — AD-01 isProductionPort + DB-name firewall), reviewed PASS,
   and routed to deployer with `needs-deploy`. After be-c5p lands on
   `origin/main`, this be-jdoy bead can re-enter the gate cleanly.
2. **Re-scope be-xtf docs.** If be-c5p is not going to ship in this
   cycle, narrow the TESTING.md section to only the symbols that
   currently exist on `origin/main` (e.g., document `BEADS_TEST_MODE`
   and `isTestDatabaseName` only; defer the `BEADS_TEST_SERVER` /
   `isProductionPort` paragraph until those ship).
3. **Combine be-c5p + be-xtf into one PR.** Have the builder produce a
   merged commit (or a 2-commit branch) that ships the firewall code
   AND the docs together as a single deploy unit. This needs a fresh
   review bead covering both pieces.

Recommendation: option (1) is the cleanest path forward — the
firewall change (be-c5p) is already implemented and on
`be-vzu-rebase-fix`, just unreviewed.

## Hand-off

- be-jdoy routed back to `beads/builder` per FAIL playbook.
- No branch pushed, no PR opened.

# PG-backend ship gate (formerly be-nwwe75 — bead vanished)

**Status:** active — DEFER protocol in effect
**Last refreshed:** 2026-05-12 by beads/pm (be-dqsu follow-up entry added; deployer escalation acknowledged + deferred)
**Origin:** mayor scope 2026-05-02 — PG backend LOCAL-ONLY until benchmarks land
**Original tracker bead:** be-nwwe75 (vanished mid-session per known bd phenomenon)

## Why this doc exists

`be-nwwe75` is the canonical tracker bead for the PostgreSQL-backend ship
gate. It keeps vanishing under the bd embedded/city-shared dolt sync
issue (see persistent memory
`bead-vanishing-phenomenon-2026-05-11-beads-created`). The agents that
need to read the ship-gate state (reviewer, deployer, builder) can't
rely on `bd show be-nwwe75` resolving. This doc is the stable
human-readable mirror; mail (gm-icydmd, gm-y9344u, gm-vrtg0h, gm-cx06pl,
gm-ll80xk, gm-7bkkz8) is the cross-agent durable channel.

## The policy

**PG-backend code does not ship to `origin/main` until the mayor
greenlights via benchmarks.** The mayor's benchmark gate (be-m89c7o,
be-ikvc3q) is the unblocker. Until then:

1. Every PG-backend commit is reviewed-clean and held on its source
   branch (`feat/be-u0zlsq-iter-interface`,
   `fix/be-efr0tm-pg-delete-by-source-repo`, and downstream fix
   branches).
2. Cherry-picking individual PG commits onto `origin/main` is
   **structurally impossible** — they depend on files introduced by
   `be-6fk.3` (PostgresBackend core, commit `691713d02`) which is
   unmerged. See `be-o54l-gate.md`, `be-mbgyxt-gate.md`,
   `be-g8cvai-gate.md`, `be-3l376t-gate.md` (all in
   `beads/deployer/release-gates/`) for proof.
3. The eventual ship vehicle is **either** a single direct PR from
   `feat/be-u0zlsq-iter-interface` **or** an explicit rollup-ship bead
   that bundles every reviewed-PASS PG commit in dependency order. Not
   decided yet — pending mayor's benchmark greenlight.

## Routing rules (per agent)

### reviewer

**Trigger conditions** (any one is sufficient — applies to BOTH
verdict-tracker beads AND proper review beads with a PASS verdict):

- Review bead carries label `area:postgres`, OR
- Reviewed commit touches files under any of:
  - `internal/storage/postgres/**`
  - `cmd/bd/doctor/postgres*`
  - `cmd/bd/init_postgres.go`
  - any other file that does not yet exist on `origin/main`

**Do NOT apply the `needs-deploy` label.** Do not route to deployer.
This applies whether you framed the bead as a "verdict tracker" or as
a "ready for release gate" review — the gate is closed for all PG-area
code, regardless of how the bead is framed. Instead:

1. Land your PASS verdict on the review bead's notes (or in mail if
   the bead vanishes).
2. Mail PM with a one-line "append to Tier N: `<bead-id> | <commit>`"
   so PM updates the "Reviewed-clean queue" section below. PM owns
   this doc.
3. Close the review bead with reason `"PG-backend — deferred per
   ship gate, see docs/plans/pg-backend-ship-gate.md"`.
4. If the source bead vanished and you used a verdict-tracker bead
   (e.g. be-o54l, be-3l376t), preserve the verdict in mail to PM.

**REQUEST CHANGES / FAIL** still routes back to builder normally. The
DEFER rule only applies to PASS verdicts on PG-area code.

### deployer

If a PG-area review bead arrives with `needs-deploy` (because the
reviewer hasn't adopted the new policy yet, or because of an in-flight
race), apply the standard DEFER disposition without escalating to PM:

1. Run the cherry-pick-conflict verification (same as
   `be-g8cvai-gate.md` for evidence).
2. Write a release-gates file with the standard FAIL verdict.
3. Close the bead with reason `"deferred per ship gate, see
   docs/plans/pg-backend-ship-gate.md"`.
4. Add the commit to the queue below (mail PM with the entry).

You do not need to escalate to PM for each new PG-area bead. The
policy is in effect until you receive explicit PM mail saying
"ship gate lifted." The PM directive from gm-icydmd (2026-05-11)
remains in force.

### builder

Continue building PG-backend work normally on the source branches.
The ship gate is downstream of you — your work lands when the rollup
ships. Do not attempt to rebase PG-area work onto `origin/main`; it
is structurally impossible (see policy point 2).

### architect

Future PG-area design beads should reference this ship-gate doc in
their notes so PM doesn't have to re-trace the routing rule each
time a new bead lands.

### pm (self)

Refresh the "Reviewed-clean queue" below as new PASS verdicts come
in via mail. When mayor signals benchmark greenlight, draft the
rollup-ship bead (or coordinate the direct-PR alternative) using
the queue as the CHERRY_PICKS source of truth.

## Reviewed-clean queue (cherry-pick order)

Order matters: each commit may depend on files introduced by an
earlier commit. This list is sourced from `be-o54l-gate.md` (PM
verified 2026-05-11) plus deltas from the gates that landed since.

**Tier 0: PG-backend core scaffold (the base everything else
depends on, on `feat/be-u0zlsq-iter-interface`)**

| Bead | Commit | Notes |
|---|---|---|
| be-u0zlsq | 19c278a48 | Iter[T] storage interface |
| be-74hkw2 | 1deba1f8b | PG infra prep |
| be-rhtega | a67427829 | PG infra prep |
| be-6fk.1 | 3e4378c05 | PostgresBackend init step 1 |
| be-6fk.2 | 85fc223f0 | PostgresBackend init step 2 |
| be-6fk.3 | 691713d02 | **PostgresBackend core** — introduces internal/storage/postgres/{config,transaction,errors}.go |
| be-b8p | c62c12ac8 | post-be-6fk.3 |
| be-6fk.5 | 2164bdac4 | |
| be-6fk.4 | b3740b997 | |
| be-6fk.6 | 0534a2c6f | |
| be-6fk.7 | 9dcf68a95 | |
| be-y8g | ea984b845 | |
| be-xz4 | f24f52192 | |
| be-2oq | cc3cc5b67 | |
| be-ry0 | e6d14d4bf | |
| be-b0h | bc9eda63a | |
| be-7yt | e99fe46da | |
| be-mlavwq | 9066444ce | |
| be-flo2kl | 940d4e68f | RunPostgresHealthChecks (4 checks) |

**Tier 1: be-yl8wc4 series (custom-types sync, on
`feat/be-u0zlsq-iter-interface`)**

| Bead | Commit | Review verdict |
|---|---|---|
| be-yl8wc4.1 | 9aa59b38f | PASS via gm-iwkwcd |
| be-d4b7dz | a4cc25447 | |
| be-g4x0eq | 80abd69b6, 4af4503f6 | PASS via gm-fhtbhv (sslmode security fix) |
| be-yl8wc4.2 | b1cf8f6de | PASS via gm-gsmw8n |
| be-yl8wc4.3 | 026550e76 | PASS via gm-vrtg0h |
| be-yl8wc4.4 | 9bfc3eb41 | No PASS verdict on file |
| be-yl8wc4.5 | 2dc146ebe | No PASS verdict on file |
| be-x7vuyp | 7c246a116 | Re-review PASS via beads/reviewer (Opus 4.7); gate at `release-gates/be-x7vuyp-gate.md` — be-g4x0eq HIGH password-leak follow-up (URI query-param redaction) |
| be-llz8 (be-qp10z7) | 4f344d5e0 | PASS via beads/reviewer (Opus 4.7); duplicate verdict-tracker for vanished be-qp10z7; gates at `release-gates/be-qp10z7-gate.md` + `release-gates/be-llz8-gate.md` — docs(migrate) Godoc + POSTGRES-BACKEND.md |
| be-7fmvcx | 3d79e00fc | No PASS verdict on file (deployer-flagged in gm-3fkvvr; sibling tracked in be-llz8 gate as "already deferred") — docs MIGRATION-RECOVERY.md + AUDIT_TRAIL cross-link |
| be-yl8wc4.6 | 383d94d75, ec93e1854 | PASS (docs only) — gated by be-mbgyxt |
| be-dqsu (be-0d4 + be-g4x0eq follow-up) | aadea37d9, 29713b231 | PASS via beads/reviewer (Opus 4.7) — on `fix/be-0d4-postgres-init-guards` off `local-integ@a67427829`. aadea37d9 = test coverage for `guardPostgresInit` dolt-refusal + `--shared-server` flag (cmd/bd/init_*); 29713b231 = sslmode URI round-trip preservation in `internal/storage/postgres/dsn/strip.go` (replaces silent pgconn TLSConfig downgrade). Deployer-verified cherry-pick conflict on origin/main; standard DEFER applies. |
| be-bsaj (cmd/bd canonical-build unblock) | b589f9b63, afab99057 | PASS via gm-9lfo5r — on `fix/be-0d4-postgres-init-guards`. b589f9b63 = test_helpers surgical delete (no-op on origin/main; **may be SKIPPED at rollup time** if PG infra doesn't reintroduce the duplicate-symbol state on the rollup branch). afab99057 = orphan test cleanup; **only ships if be-xz4 (Tier 0) ships**, since the orphan-test premise depends on be-xz4 deleting `applyBootstrapMetadataRepair` / `resolveBootstrapAuthoritativeMetadata` (still live on main). Gate: `release-gates/be-bsaj-gate.md`. Standard DEFER applies. |
| be-n1od (seedCorpus wisps coverage) | 006ce3ff1 | PASS via gm-31tys7 — on `fix/be-0d4-postgres-init-guards`. Migration roundtrip coverage; modify/delete conflict — `internal/storage/migration/` directory absent from origin/main (introduced by be-6fk.5 / 2164bdac4, Tier 0). **Lands AFTER be-6fk.5 at rollup time.** Live integration_pg run still tracked by be-f90e. Gate: `release-gates/be-n1od-gate.md`. Standard DEFER applies. |
| be-8qo7 (be-5954, POSTGRES-BACKEND.md skeleton + §2/§6) | f9833070b | PASS via gm-9rbahx — on `fix/be-0d4-postgres-init-guards`. 229 lines, new file (POSTGRES-BACKEND.md skeleton + §2/§6 verbatim). Cherry-pick conflict on origin/main (file absent). Gate: `release-gates/be-8qo7-gate.md` on branch `release/be-8qo7`. Standard DEFER applies. **First in the POSTGRES-BACKEND.md doc chain (be-8qo7 → be-76gb → be-ohoi → be-jhho → be-9myv → be-xeh1).** |
| be-76gb (be-9e34, POSTGRES-BACKEND.md §1/§3/§4 fill) | 25241ae80 | PASS via gm-u2ckiw — on `fix/be-0d4-postgres-init-guards`. +48 lines (POSTGRES-BACKEND.md §1/§3/§4 fill). Modify/delete conflict on origin/main (depends on be-8qo7). Gate: `release-gates/be-76gb-gate.md` on branch `release/be-76gb`. Standard DEFER applies. |
| be-ohoi (be-xush, POSTGRES-BACKEND.md §5/§7/§11 fill) | d312484c3 | PASS via gm-d513ay — on `fix/be-0d4-postgres-init-guards`. +126 lines (POSTGRES-BACKEND.md §5/§7/§11 fill). Modify/delete conflict on origin/main (depends on be-8qo7+be-76gb). Gate: `release-gates/be-ohoi-gate.md` on branch `release/be-ohoi`. Standard DEFER applies. |
| be-jhho (be-7en1.4, POSTGRES-BACKEND.md §8/§9/§10/§12 fill) | d5dc18b3f | Initial review had §9 F1 request-changes (per-prefix counter scope factual error); F1 fix landed as be-9myv (62a2a5d2f) and re-reviewed PASS via gm-7zw4r1 — on `fix/be-0d4-postgres-init-guards`. Fills §8 Migration + §9 Gotchas + §10 Troubleshooting + §12 See Also (handleCrossBackendMigrate at cmd/bd/migrate.go:291, defaultMaxConns=10 at open.go:19, troubleshooting wording at init_postgres.go:194). Modify/delete conflict on origin/main (depends on be-8qo7). F1 fix lands as be-9myv (62a2a5d2f) immediately below at rollup. Gate: `release-gates/be-9myv-gate.md` covers the F1 commit (the parent commit d5dc18b3f cherry-picks via the same conflict pattern as siblings). Standard DEFER applies. |
| be-9myv (be-jhho §9 F1 fix) | 62a2a5d2f | PASS via gm-7zw4r1 — on `fix/be-0d4-postgres-init-guards`. Single-paragraph §9 Concurrency rewrite per-prefix counter scope (+8/-6 on docs/POSTGRES-BACKEND.md only). Modify/delete conflict on origin/main (depends on be-8qo7 + be-jhho parent d5dc18b3f). Cherry-pick order at rollup: append 62a2a5d2f AFTER d5dc18b3f, BEFORE eb180387f (be-xeh1 stitch). Gate: `release-gates/be-9myv-gate.md` on branch `release/be-9myv` (commit 3879bd56f). Standard DEFER applies. **Clears the informational HOLD MERGE flag on be-xeh1 — §9 factual error resolved.** |
| be-xeh1 (be-7en1.5 stitch, POSTGRES-BACKEND.md TOC + anchor) | eb180387f | PASS via gm-hvtlg6 — on `fix/be-0d4-postgres-init-guards`. +16/-1 lines (TOC + one anchor fix). Modify/delete conflict on origin/main. Gate: `release-gates/be-xeh1-gate.md` on branch `release/be-xeh1`. **Last in the POSTGRES-BACKEND.md doc chain. HOLD MERGE flag cleared 2026-05-12 by be-9myv F1 fix (re-review PASS via gm-7zw4r1).** Standard DEFER applies. |

**Tier 2: be-efr0tm series (PG delete-by-source-repo, on
`fix/be-efr0tm-pg-delete-by-source-repo`)**

| Bead | Commit | Review verdict | Source gate |
|---|---|---|---|
| be-j72zr5 | 7562bcfd5 | PASS via be-g8cvai | be-g8cvai-gate.md |
| be-kil3so | 6890ae428 | PASS via be-v5e973 (gm-ll80xk) | reviewer-only (not routed) |
| be-lj68rq | a5fa7a441 | PASS via gm-u54lgr (2026-05-12, claude opus 4.7); 2nd PASS via gm-7b4d7g (review bead be-s1fl3n, closed DEFER) | be-rz20-gate.md |
| be-3ppzq7 / be-3l376t | 5120ce02f | PASS via gm-cx06pl | be-3l376t-gate.md |
| be-gzztuj | ddad18980 | PASS via gm-8hk29t (review bead be-rdvqkn, closed DEFER) | (no deployer gate — reviewer-only per ship-gate policy) |

**Tier 2 notes:**
- be-lj68rq introduces migration `0004` (ON UPDATE CASCADE on six issues-side FKs); three documented defensible deviations from be-ptzlki §6 (migration slot 0003→0004, bare ADD CONSTRAINT, literal `'renamed'` event_type). Per deployer gm-9py419: lands above the prior reviewed-clean PG slice and introduces migration 0004 — cherry-pick order: after migrations 0001–0003 are present.
- be-gzztuj implements `PromoteFromEphemeral` on PG with 7 integration_pg cases. Per reviewer gm-8hk29t: migration 0004 must precede any UpdateIssueID exercise; idempotent (DROP IF EXISTS + ADD). Follow-up `be-2bvr4u` filed for design case 6 (poisoned-row savepoint rollback) — non-blocking, testable via wisp_events.actor NULL → events.actor NOT NULL violation.
- With be-gzztuj reviewed-clean, **CP-3 BulkIssueStore lifecycle parity epic (be-x1kg5f) is complete** — all 4 method beads PASS. The parity rollup test bead `be-jygyks` is now unblocked and feeds the ship-gate directly. One of 5 PG ship-gate blockers satisfied; 4 remain (soak/be-ry0mig, benchmarks/be-5upx19, parity rollup/be-jygyks, doctor parity/be-4i7ax3).

**Notes:**
- Tier 0 ordering predates this PM seat's view; the architect should
  verify on rollup-day.
- be-yl8wc4.4, be-yl8wc4.5, and be-7fmvcx have no PASS verdict
  visible to PM — flag for reviewer attention if those commits are
  still in scope at rollup time.
- be-x7vuyp (7c246a116) is the HIGH password-leak follow-up to the
  be-g4x0eq pair (80abd69b6, 4af4503f6). Per
  `release-gates/be-x7vuyp-gate.md`, ship as a triple in branch
  order: TDD pin → fix → password-redaction strengthening.
- be-llz8 (4f344d5e0) Godoc targets `handleCrossBackendMigrate`,
  introduced by be-6fk.5 (commit 2164bdac4, `bd migrate --to=postgres`).
  Architect should verify final Tier 1 cherry-pick order at rollup
  time — Tier 0's be-6fk.5 must precede this docs commit, which is
  satisfied by the existing Tier 0 → Tier 1 ordering but worth
  re-checking when the rollup-ship bead is assembled.

## What changed today (2026-05-12)

**Latest update (gm-nkbcha from deployer):** POSTGRES-BACKEND.md
doc chain §9 F1 fix landed. be-9myv (62a2a5d2f) is the re-review
PASS of the §9 factual error originally request-changed against
be-jhho's §8/§9/§10/§12 fill; re-review via gm-7zw4r1 on
`fix/be-0d4-postgres-init-guards`. Standard PG-area DEFER applied
by deployer (no PM round-trip needed per gm-b01bf6). PM-side
updates:
- Added the missing be-jhho row to Tier 1 (commit d5dc18b3f) —
  this row was absent from earlier updates because be-jhho was
  request-changed at first review; with the F1 fix passing, the
  parent commit now ships as part of the rollup chain.
- Added be-9myv row right after be-jhho (commit 62a2a5d2f) per
  the deployer's queue-entry instruction.
- Cleared the informational HOLD MERGE flag on the be-xeh1 row
  (§9 factual error is now resolved, doc ships clean as a unit).
- Updated the chain note above to reflect the six-commit
  cherry-pick order at rollup time.

**Earlier update (gm-1fvlqf from deployer):** POSTGRES-BACKEND.md user
doc chain (4 commits) appended to Tier 1, all PASS + DEFER per the
standard ship-gate rule (no PM round-trip per gm-b01bf6 — deployer
self-disposed correctly). All four on `fix/be-0d4-postgres-init-guards`;
all four hit cherry-pick conflicts off origin/main (file absent /
modify-delete). The doc builds up linearly:
- `be-8qo7` (be-5954) / `f9833070b` (PASS via gm-9rbahx; gate
  `release-gates/be-8qo7-gate.md` on `release/be-8qo7`) —
  POSTGRES-BACKEND.md skeleton + §2/§6 verbatim (229 lines, new file).
- `be-76gb` (be-9e34) / `25241ae80` (PASS via gm-u2ckiw; gate
  `release-gates/be-76gb-gate.md` on `release/be-76gb`) —
  POSTGRES-BACKEND.md §1/§3/§4 fill (+48 lines).
- `be-ohoi` (be-xush) / `d312484c3` (PASS via gm-d513ay; gate
  `release-gates/be-ohoi-gate.md` on `release/be-ohoi`) —
  POSTGRES-BACKEND.md §5/§7/§11 fill (+126 lines).
- `be-xeh1` (be-7en1.5 stitch) / `eb180387f` (PASS via gm-hvtlg6 with
  **HOLD MERGE flag** — informational; gate
  `release-gates/be-xeh1-gate.md` on `release/be-xeh1`) —
  POSTGRES-BACKEND.md TOC + one anchor fix (+16/-1 lines).

Rollup order is linear (commit-level): f9833070b → 25241ae80 →
d312484c3 → d5dc18b3f → 62a2a5d2f → eb180387f
(be-8qo7 → be-76gb → be-ohoi → be-jhho → be-9myv F1 fix → be-xeh1).
HOLD MERGE flag on be-xeh1 was cleared 2026-05-12 by the be-9myv
F1 fix re-review PASS (gm-7zw4r1). All release branches pushed to
fork; all beads closed with the standard ship-gate-defer reason.

**Earlier update (gm-udgrwe from deployer):** Two PG-derivative entries
appended to Tier 1, both PASS + DEFER per the standard ship-gate rule
(no PM round-trip per gm-b01bf6 — deployer self-disposed correctly):
- `be-bsaj` / `b589f9b63`, `afab99057` (PASS via gm-9lfo5r; gate
  `release-gates/be-bsaj-gate.md`) — cmd/bd canonical-build unblock
  via test_helpers surgical delete + orphan test cleanup. Rollup-day
  routing: `b589f9b63` is a no-op on origin/main (duplicate-symbol
  state does not exist there) and may be SKIPPED if PG infra doesn't
  reintroduce that state on the rollup branch; `afab99057` only ships
  if be-xz4 (Tier 0) ships, since the orphan-test premise depends on
  be-xz4 deleting `applyBootstrapMetadataRepair` /
  `resolveBootstrapAuthoritativeMetadata`.
- `be-n1od` / `006ce3ff1` (PASS via gm-31tys7; gate
  `release-gates/be-n1od-gate.md`) — seedCorpus wisps coverage for
  migration roundtrip. Modify/delete conflict —
  `internal/storage/migration/` directory absent from origin/main
  (introduced by be-6fk.5 / 2164bdac4, Tier 0). Lands AFTER be-6fk.5
  at rollup time. Live integration_pg run still tracked by be-f90e.

Both on `fix/be-0d4-postgres-init-guards` (same branch as be-dqsu).
Deployer applied the full DEFER bookkeeping and queue-entry mail; no
PM action other than this doc update.

**Earlier update (be-dqsu deployer escalation, gate FAIL re-route):**
Deployer escalated `be-dqsu` (be-0d4 postgres-init guards + be-g4x0eq
sslmode round-trip fix; reviewer PASS; commits `aadea37d9` +
`29713b231` on `fix/be-0d4-postgres-init-guards`) with the structural
cherry-pick-conflict FAIL — same architectural blocker pattern as
be-rhtega. Per this doc's deployer routing rule (line 73–88), this
escalation was unnecessary: deployer should apply the standard DEFER
without escalating. PM has appended the entry to Tier 1 above, will
mail deployer a reminder of the routing rule, and is closing the
bead with the standard ship-gate-defer reason.

**Earlier update (gm-3fkvvr from deployer):** Tier 1 backfill — three
commits that were absent from the Reviewed-clean queue have been
added between be-yl8wc4.5 and be-yl8wc4.6 in branch order:
- `be-x7vuyp` / `7c246a116` (re-review PASS; gate
  `release-gates/be-x7vuyp-gate.md`) — be-g4x0eq HIGH password-leak
  follow-up.
- `be-llz8` (be-qp10z7) / `4f344d5e0` (PASS, second DEFER; gates
  `release-gates/be-qp10z7-gate.md` + `release-gates/be-llz8-gate.md`)
  — docs(migrate) Godoc + POSTGRES-BACKEND.md. Cherry-pick verified
  to conflict on `cmd/bd/migrate.go` (targets
  `handleCrossBackendMigrate`, absent from origin/main `da73b7511`).
- `be-7fmvcx` / `3d79e00fc` (no PASS verdict on file; deferred per
  be-llz8 gate "sibling already deferred" note) — docs
  MIGRATION-RECOVERY.md + AUDIT_TRAIL cross-link.

The previous deployer (2026-05-11) had also requested the be-qp10z7
entry and the PM seat at the time missed it — apologies for the
double-routing. Going forward the deployer's DEFER pattern with a
mail-to-PM queue update is the authoritative loop; PM updates the
doc and acks via mail.

**Late-evening update (gm-7b4d7g + gm-8hk29t from reviewer):** CP-3
BulkIssueStore lifecycle parity epic (be-x1kg5f) is COMPLETE — all 4
method beads carry PASS verdicts:
- be-kil3so (DeleteIssues) — PASS via be-v5e973 (gm-ll80xk)
- be-efr0tm (DeleteIssuesBySourceRepo) — PASS via be-g8cvai
- be-lj68rq (UpdateIssueID) — PASS via gm-u54lgr + gm-7b4d7g
- be-gzztuj (PromoteFromEphemeral) — PASS via gm-8hk29t (newly added)

The parity rollup test bead `be-jygyks` is now actionable (was blocked
on all 4 builders). Filed follow-up: `be-2bvr4u` (needs-tests) for
poisoned-row savepoint rollback — non-blocking.

**Earlier today:** The deployer's gm-7bkkz8 surfaced that the per-bead
DEFER pattern is working (4 deferrals on 2026-05-11, 1 today) but
routes through deployer every time. The reviewer-side policy in this
doc closes that loop: PG-area PASS verdicts should record straight to
this doc and close the bead, no `needs-deploy` label.

The PM-side directive in gm-icydmd (2026-05-11) remains in force for
the case where a `needs-deploy` PG-area bead still lands at deployer
(in-flight race or reviewer hasn't picked up the new rule yet):
deployer just DEFERs without escalating.

## Outstanding decisions

1. **Ship vehicle** — direct PR from `feat/be-u0zlsq-iter-interface`
   vs. rollup-ship bead with CHERRY_PICKS list. Mayor picks at
   benchmark greenlight (gm-73ypk1, 2026-05-12). Both options
   acknowledged valid; no PM preference.
2. **be-yl8wc4.4 / be-yl8wc4.5 review status** — flag to reviewer at
   rollup time so no commits ship un-reviewed.
3. **be-rz20 (PG UpdateIssueID)** — open at deployer right now with
   `needs-deploy` label. PM is mailing deployer to apply the
   standard DEFER disposition, no PM round-trip needed for the
   subsequent ones.

## Greenlight timeline (per mayor gm-73ypk1, 2026-05-12)

Mayor target: **~evening of 2026-05-12** if validator's headless
harness (mc-3rph5h, PASSED 2026-05-11 night) holds up under a real
claude-plays-mcdclient playtest on 2026-05-12. The mcdclient story
is gating the PG benchmark greenlight; until that lands cleanly, PG
deploy work stays deferred.

This is a soft target, not a commitment. PM should not pre-stage the
rollup-ship bead until mayor's explicit greenlight mail arrives.

## Source documents

- Mail thread: gm-icydmd, gm-y9344u, gm-vrtg0h, gm-cx06pl, gm-ll80xk,
  gm-7bkkz8, gm-fhtbhv, gm-gsmw8n, gm-iwkwcd, gm-3fkvvr
- Release gates: `beads/deployer/release-gates/{be-o54l, be-mbgyxt,
  be-g8cvai, be-3l376t, be-ocb20w, be-qp10z7, be-llz8, be-x7vuyp,
  be-rz20}-gate.md`
- Bead notes (when they resolve): be-nwwe75, be-o54l

# Release gate: be-746/be-ec5 — cfg.GetDoltDatabase() env-hijack fix + resolveDoltSourceDataDir tests

**Verdict: PASS.**

Branch: `fix/be-746-migrate-env-hijack`
Base branch: `users/jaword/postgres-backend` (stacked PR — postgres backend feature branch)
HEADs: `efef5ef77` (tests, be-ec5), `35a151a5a` (fix, be-746)
Source beads: `be-746`, `be-ec5`; review bead: `be-7dk5pe` (PASS).

## Commits

| # | SHA | Subject |
|---|-----|---------|
| 1 | `efef5ef77` | test(cli): be-ec5 cover resolveDoltSourceDataDir branches |
| 2 | `35a151a5a` | fix(migrate): be-746 — read source DB from metadata.json, not env var |

Files changed: `cmd/bd/migrate_resolve_test.go` (187 insertions), `cmd/bd/migrate.go` (4 insertions, 2 deletions).

## Criteria

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | Review PASS present | **PASS** | be-7dk5pe notes: "VERDICT: pass. Findings: none." Commits efef5ef77 and 35a151a5a reviewed. |
| 2 | Acceptance criteria met | **PASS** | Fix changes `cfg.GetDoltDatabase()` → `cfg.DoltDatabase` in `resolveDoltSourceDataDir`, removing BEADS_DOLT_SERVER_DATABASE env influence. `TestResolveDoltSourceDataDir_IgnoresServerDatabaseEnv` (previously failing as 6/7) now PASS. All 7 resolveDoltSourceDataDir tests pass. |
| 3 | Tests pass | **PASS** | `go test -tags gms_pure_go -count=1 -run TestResolveDoltSourceDataDir ./cmd/bd/...`: ok (0.162s, 7/7 pass). |
| 4 | No high-severity review findings open | **PASS** | Reviewer be-7dk5pe: "Findings: none." |
| 5 | Final branch is clean | **PASS** | `git status` clean (untracked `.gc/`, `.gitkeep` are rig scaffolding). |
| 6 | Branch diverges cleanly from base | **PASS** | 2 commits ahead of `users/jaword/postgres-backend`. `git merge-tree` shows zero conflicts. |

## Stacking note

`fix/be-746-migrate-env-hijack` is stacked on `users/jaword/postgres-backend` (the PG backend feature branch). The base branch is pushed to fork (`quad341:users/jaword/postgres-backend`) so the PR diff shows only the 2 fix/test commits.

## Test environment

- Host: Linux 6.19.14-300.fc44.x86_64

## Push target

`PUSH_REMOTE=fork`. PR opened within fork: `quad341:fix/be-746-migrate-env-hijack` → `quad341:users/jaword/postgres-backend`.

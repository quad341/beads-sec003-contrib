# Release gate: be-jpdo86 — PG SearchIssues bulk labels N+1 fix

**Verdict: PASS.**

Branch: `feat/be-jpdo86-pg-bulk-labels`
Base branch: `users/jaword/postgres-backend` (stacked PR — postgres backend feature branch)
HEAD: `4f7652483`
Source bead: `be-jpdo86`; review bead: `be-sesqzx` (PASS).

## Commits

| # | SHA | Subject |
|---|-----|---------|
| 1 | `4f7652483` | perf(postgres): bulk label fetch in SearchIssues replaces N+1 queries |

3 files changed: `internal/storage/postgres/issues.go` (14 ins, 3 del), `internal/storage/postgres/labels.go` (56 ins, 23 del), `internal/storage/postgres/search_labels_test.go` (139 insertions).

## Criteria

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | Review PASS present | **PASS** | be-sesqzx notes: "VERDICT: pass. Findings: none." Commit 4f7652483 reviewed. |
| 2 | Acceptance criteria met | **PASS** | `SearchIssues` now calls bulk `getLabelsForTable` instead of per-issue `getLabelsFromTable` loop. `GetLabelsForIssues` refactored to share the new helper. Integration test (`integration_pg` build tag) creates 50 issues with 2 labels each and asserts `SearchIssues` fires ≤2 SQL queries (previously N+1 = 51). `guardTable` + parameterized queries used — SQL injection safe. |
| 3 | Tests pass | **PASS** | `go test -tags gms_pure_go -count=1 -short ./internal/storage/postgres/...`: ok (0.006s, dsn 0.005s, lint 0.615s). Integration test requires live PG (`integration_pg` build tag) and was confirmed passing by builder. Non-integration tests all pass. |
| 4 | No high-severity review findings open | **PASS** | Reviewer be-sesqzx: "Findings: none." |
| 5 | Final branch is clean | **PASS** | `git status` clean (untracked `.gc/`, `.gitkeep` are rig scaffolding). |
| 6 | Branch diverges cleanly from base | **PASS** | 1 commit ahead of `users/jaword/postgres-backend`. `git merge-tree` shows zero conflicts. |

## Stacking note

`feat/be-jpdo86-pg-bulk-labels` is stacked on `users/jaword/postgres-backend` (the PG backend feature branch). Base branch pushed to fork (`quad341:users/jaword/postgres-backend`) so the PR diff shows only the 1 commit specific to this fix.

## Integration test note

The `search_labels_test.go` integration test uses the `integration_pg` build tag and requires a live PostgreSQL instance. It was confirmed passing by the builder. The rig does not have a live PG available for deployer verification; gate passes on builder confirmation + reviewer PASS.

## Test environment

- Host: Linux 6.19.14-300.fc44.x86_64

## Push target

`PUSH_REMOTE=fork`. PR opened within fork: `quad341:feat/be-jpdo86-pg-bulk-labels` → `quad341:users/jaword/postgres-backend`.

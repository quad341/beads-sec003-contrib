# PR Triage: 11 Open Factory PRs vs coffeegoddd's Review Rubric

**Root bead**: be-v1bz65  
**Triage date**: 2026-05-12  
**Rubric source**: bd memory `filing-prs-upstream-coffeegoddd-rubric`

## Rubric (5 rules)

1. **Beads-only repro**: prove the bug with stock beads + no orchestrator
2. **Single layer per PR**: if a fix spans layers, stack PRs
3. **Primitive first**: storage/issueops methods first, application calls them
4. **Minimize inline comments**: remove them; let commit message carry the why
5. **Stock beads first**: reproduce with no orchestrator running before claiming a bug

---

## Triage Decisions

### #3458 — perf(storage): SearchIssueSummaries narrow-projection list
**Review**: "too complicated, too many layers at once, start from issueops"  
**Status**: Author already responded (2026-05-08 commit `dc9f9769`) — pushed narrow-projection into issueops, SQL-side sort, removed Go-side sort. PR is waiting for coffeegoddd re-review.  
**Decision**: ✅ REVISE COMPLETE — request re-review.  
**Action bead**: A

### #3461 — perf(storage): CountIssues / CountIssuesGroupedBy
**Review**: "touching too many layers, start from issueops, get benchmark"  
**Status**: PR has storage-layer methods BUT also changes cmd/bd/count.go in same PR. Stacks on #3458.  
**Decision**: CLOSE + refile as 2 PRs: (1) issueops CountIssues primitive + storage interface only, (2) wire cmd/bd/count.go once primitive lands.  
**Action bead**: D

### #3526 — perf(issueops): replace per-id wisp check in 3 bulk fetchers
**Review**: "needs benchmarks demonstrating faster + correct results. Delete all code comments."  
**Status**: Narrow focused PR (3 bulk fetchers at issueops layer). Single layer ✓. Just missing benchmarks + has inline comments.  
**Decision**: ✅ REVISE — add benchmarks, strip inline comments.  
**Action bead**: C

### #3540 — perf(storage): gate compat migrations with tracking table
**Review**: "remove all compat_migrations/dolt/migrations, only use internal/schema going forward"  
**Status**: Author shipped a tracking-table gate; coffeegoddd wants full consolidation into internal/schema. Fundamental architecture redirect.  
**Decision**: CLOSE current PR + needs-architecture bead: "Consolidate compat_migrations into internal/schema pattern".  
**Action bead**: E

### #3662 — perf(schema): D4v2 composite (status, updated_at) + defer_until indexes
**Review**: NONE YET. Review requested from coffeegoddd. PR is clean: schema migrations only, tested, codecov 100%.  
**Decision**: WAIT / ping — nothing wrong with the PR; just needs coffeegoddd's attention.  
**Action bead**: A (re-request)

### #3710 — perf(dolt): eliminate 12s slow path on bd commands
**Review**: "need beads-only repro — what versions, what bd commands reproduce the 12s?"  
**Status**: No repro provided. Rubric hits #1 + #5.  
**Decision**: REVISE — add beads-only repro script (exact binary versions, exact bd command sequence, measured latency).  
**Action bead**: B

### #3734 — fix(close): refuse silent close on actor/assignee mismatch
**Review**: "add minimal repro of race + data loss. What if different actors have same name?"  
**Status**: No repro. Also unresolved design question about same-name actors.  
**Decision**: REVISE — add beads-only repro + address same-name case in PR description.  
**Action bead**: B

### #3768 — fix(show): bound bd show --json RAM
**Review**: "13K lines, missing simple repro or benchmark"  
**Status**: Massive diff. Rubric hits #1 (no repro), #2 (too large).  
**Decision**: CLOSE + refile as: (1) minimal beads-only repro script showing OOM/RAM growth, (2) targeted fix only (strip heavy fields from Dependents, benchmark showing before/after).  
**Action bead**: F

### #3780 — schema: migration progress + bd list filter regression tests
**Review**: "What is the goal? Orchestrator should not kill process assuming hung — logs won't help if timeout fires anyway."  
**Status**: coffeegoddd's point is valid: the orchestrator-timeout problem is not fixed by stderr output. The human UX (^C during 25-75s migration) is a narrower, valid goal.  
**Decision**: CLOSE + refile as narrow PR: "print one-line migration-in-progress to stderr" targeting human UX only; explicitly out of scope: orchestrator timeout fixes.  
**Action bead**: G

### #3813 — fix(utils): be-szr ResolvePartialID falls through to GetIssue for wisps (PG)
**Review**: "need beads-only minimal repro, otherwise this is an orchestration-layer bug"  
**Status**: PG-backend specific fix. Without repro, coffeegoddd won't know it's beads vs orchestrator. Rubric hits #1 + #5.  
**Decision**: REVISE — add beads-only repro (no orchestrator, just stock bd + PG backend).  
**Action bead**: B

### #3351 — perf(export): incremental auto-export via dolt_diff
**Review**: "add DoltDiff primitive to storage.DoltStorage first, then build ChangedIssueIds on top"  
**Status**: Primitive-first pattern violation (rubric #3). The change skips the primitive layer.  
**Decision**: CLOSE current PR + refile as 2 PRs: (1) add `DoltDiff(fromCommit, toCommit) ([]string, error)` primitive to storage.DoltStorage, (2) build ChangedIssueIds using that primitive.  
**Action bead**: H

---

## Action Beads Created

| Bead | Action | PRs | Route |
|------|--------|-----|-------|
| A | Re-request review (already fixed / awaiting) | #3458, #3662 | builder |
| B | Add beads-only repro scripts | #3710, #3734, #3813 | builder |
| C | Add benchmarks + strip inline comments | #3526 | builder |
| D | Close + refile as 2 layered PRs | #3461 | builder |
| E | Close + needs-architecture for compat migration consolidation | #3540 | architect |
| F | Close 13K-line PR + refile focused RAM fix | #3768 | builder |
| G | Close + refile narrow migration stderr indicator | #3780 | builder |
| H | Close + refile with DoltDiff primitive first | #3351 | builder |

---

## Key Pattern

Eight of eleven PRs hit rubric #1 (no beads-only repro) or rubric #2/#3 (multi-layer). The factory's standard workflow needs to add a "repro first" gate before pushing any fix PR: a beads-only `TestBug_XYZ` or benchmark must exist before the fix goes upstream.

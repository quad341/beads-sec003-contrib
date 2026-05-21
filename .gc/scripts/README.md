# City-level shared scripts

This directory holds shell scripts that are invoked by multiple
agent packs (or by maintainers across multiple packs). Scripts here
are referenced absolutely via the `{{.CityRoot}}` template.

## Scripts

| Script | Purpose | Invoked from |
|---|---|---|
| `worktree-setup.sh` | Create the per-agent git worktree, namespace the branch, redirect bd state, and (optionally) sync prerequisite skills. | `pre_start` in 20 `packs/actual/*/pack.toml`. |
| `sync-actual-skill.sh` | Maintainer-only: re-vendor the upstream `actual-skill` skill catalog into a pack's skill directory. | Human-run: `${GC_CITY}/scripts/sync-actual-skill.sh [REF]`. Not in any pack hook. |

## Rename / move coupling

Renaming or moving either script requires updating:

1. Every `pre_start` line in `packs/actual/*/pack.toml` that references
   the script (20 lines for `worktree-setup.sh` today).
2. The `pre-start-scripts` doctor check expectations
   (`internal/doctor/pre_start_scripts_check.go`).
3. Each `packs/actual/*/README.md` and `packs/actual/AGENT_PACK.md`
   block that documents the human-run invocation
   (6 READMEs + AGENT_PACK.md for `sync-actual-skill.sh` today).

The `pre-start-scripts` doctor check (added in PR #1778, widened for
`{{.CityRoot}}` in the PR that introduced these scripts here) will
catch broken `pre_start` references at config-load.

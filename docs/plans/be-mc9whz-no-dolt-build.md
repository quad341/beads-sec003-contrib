# Plan: be-mc9whz handoff — `-tags no_dolt` build

**Source design:** be-mc9whz (designer, 2026-05-07)
**Architecture parent:** be-ef3q3u (cold-start RAM budget — Option B)
**Owner of plan:** beads/pm
**Date:** 2026-05-07

## Goal

Add a `no_dolt` build tag that produces a `bd` binary without
`internal/storage/dolt`, `internal/storage/embeddeddolt`,
`internal/storage/doltdriver`, or any `github.com/dolthub/*`
dependency. Target ≤ 60 MiB stripped (down from ~184 MiB). Default
build remains Dolt-bundled; `no_dolt` is opt-in for PG-only
operators.

## Decomposition

Designer recommended a single PR ("file renames + new files + CI
entry MUST land together to avoid a half-state where the build
matrix is broken"). pm respected that; one implementation bead.

One bead, routed to `beads/builder` with label `ready-to-build`:

| Child | Title | Blocked-by |
|-------|-------|------------|
| be-mlavwq (P1) | Implement -tags no_dolt build for PG-only bd binary | — |

### be-mlavwq — `-tags no_dolt` implementation

Single PR per designer. Touches ~17 files (file-by-file in design
§11). Major work units:

1. **Build-tag matrix (design §2):** 3-level naming
   `*_dolt_cgo.go`, `*_dolt_nocgo.go`, `*_no_dolt.go` — explicitly
   mirrors the cgo × no_dolt 2D matrix. Renames existing `.go` /
   `_nocgo.go` files in the same PR; splitting the rename into a
   separate PR would create a half-state where the matrix doesn't
   match the file naming.

2. **doltdriver split (design §3):** `internal/storage/doltdriver/`
   3-way split. `_no_dolt.go` registers `BackendDolt` with a factory
   returning `NoDoltSupportErrMsg` — no transitive Dolt deps from
   that file.

3. **store_factory split (design §4):** `cmd/bd/store_factory.go`
   3-way split. `_no_dolt.go` variant intercepts `backend=dolt` early
   with `NoDoltBinaryMsg` (cleaner than letting the registry stub
   error stand alone).

4. **Subcommand gating (design §5):** `//go:build !no_dolt` on
   `cmd/bd/dolt*.go`, `backup_dolt*.go`, `federation*.go`. Optional
   cobra stub commands print install-guidance under `no_dolt`.

5. **beads.go split (design §6):** `_dolt.go` keeps `Open` /
   `OpenFromConfig`; `_no_dolt.go` stubs them with
   `ErrDoltUnavailable`. `OpenWithBackend` works under both.

6. **Test surface audit (design §7):** prefer `!no_dolt` exclusion
   over refactor for tests touching Dolt symbols outside `dolt_only`.

7. **CI matrix entry (design §8):** `make build-no-dolt` target plus
   GitHub Actions job that runs the smoke test (PG container +
   create/show/close) and the binary-size assertion at 60 MiB. Block
   merges on this lane.

8. **Documentation (design §10):** README, INSTALLING.md, and the
   runtime help-text emitted by the `NoDoltBinaryMsg` error.

## Functional requirements (from be-ef3q3u §2)

- **FR-02** Build under `-tags no_dolt` with `CGO_ENABLED=0|1`
- **FR-03** Runtime error (clear install-guidance), not panic, when
  `metadata.json` says `backend=dolt`
- **FR-04** Dolt-specific subcommands elided from the command tree

## Non-functional requirements

- **NFR-03** Binary ≤ 60 MiB stripped on Linux amd64
- **NFR-04** Default user-facing build still includes Dolt;
  `no_dolt` is opt-in

## Coordination

- No dependencies on other in-flight beads in this scope.
- Architect open questions in design §14 have recommended answers —
  builder applies them as defaults and surfaces departures to the
  architect via mail before merging.

## Out of scope

- Daemon mode (Option A — be-oyer9z)
- gc-side defer (Option C — different rig, gascity)
- aws-sdk-go separation (separate `no_aws` tag — follow-up bead;
  architect Q3 leans toward separate tag)

## Done-when (plan-level)

- be-mlavwq merged (single PR; file renames + new files + CI entry
  together)
- New CI job `build-no-dolt` gates merges
- Smoke test passes (`bd init --backend=postgres` end-to-end under
  `no_dolt`)
- Binary-size assertion ≤ 60 MiB stripped passes
- Default build (without `-tags no_dolt`) unchanged; existing CI
  lanes still pass
- Designer + architect notified via mail

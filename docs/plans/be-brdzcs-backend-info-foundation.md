# Plan — BackendInfo resolver + dsn helpers (be-brdzcs)

**PM:** beads/pm-1 · **Date:** 2026-05-09
**Parent architecture:** be-pbnoaa (architect, closed) · **Design:** be-brdzcs notes (beads/designer-1)
**Sibling surface beads (downstream consumers):** be-hjsr03, be-8f8esf, be-8mw29t

## Goal

Ship a single backend-resolution foundation for the truthful Postgres
backend reporting feature (be-pbnoaa). Three new files in two
packages, no user-facing CLI surface in this bead. Subsequent surface
beads consume this foundation to fix `bd context`, `bd info`,
`bd where`, `bd doctor`, `bd dolt status`, and add the new
`bd backend status` command.

## Context

`bd` and `gc` cannot truthfully report the active backend during/after
the Dolt → Postgres migration. The architect (be-pbnoaa) defined the
fix as: a single backend resolver returning `BackendInfo`, plus
defence-in-depth password-redaction helpers, that every status surface
must consult instead of hardcoding `Backend: dolt` or `mode: direct`.

This foundation bead is the resolver itself. It does not change any
user-facing output — that's the surface beads' job. Without this
landing first, the surface work cannot begin.

## PM judgements (designer escalations)

The designer flagged four open questions in §7. PM resolution:

### Q1 — Add `Socket string` field to `BackendInfo`? — APPROVED (option a)

Designer's two options:
- **(a)** Add `Socket string omitempty`, surfaces emit `socket: <path>`
  line. Scope grows by one field; no information loss.
- **(b)** Smuggle the socket into `Host` with a `unix:` scheme prefix.
  Smaller API but ergonomics suffer in `--json` consumers.

PM picks (a). Rationale:
- One-field addition is genuinely cheap.
- `--json` consumers (gc skill, observability scrapers) get a clean
  named field instead of having to detect-and-strip a scheme prefix.
- Naming matches existing `Config.DoltServerSocket`. Discoverability.
- The unix-socket case is real (test matrix §5.1 case #4); it
  deserves first-class representation.

**Builder must:** add `Socket string` with `json:"socket,omitempty"`,
populated when `Backend == dolt && Mode == server &&
cfg.GetDoltServerSocket() != ""`. Render contract: `socket:` line
between `port:` and `database:`, 13-char label width preserved.
Test §5.1 case #4 additionally asserts `Socket == "/tmp/dolt.sock"`.

### Q2 — Reserve `Source` field for future env-override values — DOCUMENTED

When env-override support lands for Postgres host/port (be-zdhe4l),
`Source` may grow values like `"metadata.json+env"`. Builder MUST NOT
enum-lock the field; treat as free-form string.

### Q3 — `gc bd doctor` cross-check — OUT OF SCOPE

Per architect §16, this is a separate work item. The
`BackendInfo.Source` field is the hook for future cross-check work.
No action this bead.

### Q4 — Performance budget on 9P mounts — BENCH IF AVAILABLE

The `os.Stat(<beadsDir>/dolt)` call in `DetectLegacyDoltState` is
cheap on warm cache but slower on WSL/9P filesystems (per existing
`GetDoltDataDir` comment at configfile.go:412). The
Postgres-only short-circuit is already locked in §3.1 — no extra
work expected. Builder benches if a 9P mount is conveniently
available; if regression detected, surface it back to PM.

## Decomposition decision: one builder bead

The architect already decomposed be-pbnoaa into four child beads:
foundation (be-brdzcs, this one) and three surface consumers
(be-hjsr03, be-8f8esf, be-8mw29t). Within the foundation, the work
divides naturally into three files (`backend_info.go`, `legacy.go`,
`render.go`) but the **test matrices cross-reference each other** —
the resolver tests exercise the dsn helpers; the legacy detection
tests share fixtures with the resolver tests. Splitting the
foundation further would create three tiny PRs that all touch the
same fixture pack and produce no parallelization win.

**One bead, one PR.** Builder owns implementation + tests inline per
Go convention.

## Validator role

Light. The designer's test matrices (§5.1–§5.4) already specify 36
test cases that meet the 100% line-coverage acceptance bar. Builder
writes them inline. Validator's role is post-build coverage
verification — confirm `go test -coverprofile` shows 100% on the
four target functions and no regressions in the broader
`internal/configfile/...` and `internal/storage/postgres/dsn/...`
packages. No separate `needs-tests` bead.

## Acceptance (from designer §9, with PM Q1 addition)

This bead lands when:

1. `internal/configfile/backend_info.go` exists with the `BackendInfo`
   struct (per §1.1 with §1.2 omitempty refinements **plus PM Q1
   `Socket string omitempty`**), `ResolveBackendInfo(beadsDir string)
   BackendInfo` per §4 control flow, godoc on every exported symbol.
2. `internal/configfile/legacy.go` exists with `DetectLegacyDoltState`
   per §3.1. Sorted output. Godoc.
3. `internal/storage/postgres/dsn/render.go` exists with
   `RenderRedacted(dsn string) (string, error)` and
   `ParseConnectionTarget(dsn string) (ConnectionTarget, error)`
   (designer-locked structured return). Both go through pgconn +
   ConfigToConnString. Errors do NOT echo input. Godoc.
4. Test files cover the matrices in §5.1–§5.4 (36 cases total) plus
   the PM Q1 Socket-field assertion in §5.1 case #4.
5. `go test ./internal/configfile/... ./internal/storage/postgres/dsn/...`
   passes.
6. `golangci-lint run ./internal/configfile/... ./internal/storage/postgres/dsn/...`
   passes (baseline tolerated per LINTING.md).
7. `go test -coverprofile` shows 100% line coverage on
   `ResolveBackendInfo`, `DetectLegacyDoltState`, `RenderRedacted`,
   `ParseConnectionTarget`.

## Non-goals (reaffirmed)

- No changes to `cmd/bd/`. Surface migration is be-hjsr03 / be-8f8esf
  / be-8mw29t.
- No new `metadata.json` fields. Resolver only reads what's persisted
  today.
- No DB connection. Resolver does not open the database, does not
  ping anything.

## Coordination notes

From the architect's parent bead (2026-05-09):

- PR#3758 touches `cmd/bd/doctor.go` (Dolt embedded mode safety) — no
  conflict with this bead, but be-8mw29t (downstream) should rebase.
- PR#3755 touches `cmd/bd/doctor/fresh_clone_server.go`,
  `legacy.go` — same. Note: this bead's `internal/configfile/legacy.go`
  is a NEW file; no conflict expected with the doctor-package
  `legacy.go`.
- PR#3833 touches `internal/storage/db/server` — no overlap.

## Routing

- `needs-design` removed (designer is done)
- `needs-pm` removed (PM is done)
- `ready-to-build` added
- `gc.routed_to` updated from `beads/pm` → `beads/builder`
- Bead reset to `open` + unassigned so builder can claim via
  `bd ready --metadata-field gc.routed_to=beads/builder --unassigned`

`needs-architecture-followup` retained — appears on all four siblings;
treat as informational epic-level marker.

---

*— beads/pm-1, 2026-05-09*

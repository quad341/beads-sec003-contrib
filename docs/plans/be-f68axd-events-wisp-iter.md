# Plan — `bd events` / `bd wisp list` to Iter (be-f68axd)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-f68axd (designer, 2026-05-15)
**Parent architecture:** be-yinl4d (RAM budget — Iter migrations)
**Dependencies:** be-jaavsb (CLOSED), be-u0zlsq (CLOSED) — IterEvents, IterAllEventsSince, IterWisps available

## Goal

Migrate 3 call sites in `bd events` and `bd wisp list` to iterator paths. No user-visible changes.

## Decomposition decision: one builder bead

3 call sites, same migration pattern. Self-contained. One PR.

Builder bead: be-ritg7o.

## Call sites (from design §2)

1. `bd events --json` when `--limit 0` → `IterEvents`
2. `bd events --since <t> --json` — **always `IterAllEventsSince`** regardless of limit. No bounded variant for full-rig event scans.
3. `bd wisp list --json` when `--limit 0` → `IterWisps`

Bounded cases (`--limit N > 0`) stay on slice path (`GetEvents`, `ListWisps`).

## JSON streaming

For `--json` paths: `[`, per-item `json.NewEncoder(w).Encode(item)`, `]`.
Same pattern as be-c40q2v.

## Acceptance (builder bead be-ritg7o)

- `bd events --json --limit 0` uses `IterEvents`.
- `bd events --since <t> --json` always uses `IterAllEventsSince`.
- `bd wisp list --json --limit 0` uses `IterWisps`.
- Bounded paths unchanged.
- `go test ./cmd/bd/... -run 'TestEvent|TestWisp'` clean.
- Bench: `bd events --since 30d` RSS < 50 MB on high-churn fixture.

## Routing

- Builder bead: be-ritg7o → `beads/builder`
- Design bead be-f68axd: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

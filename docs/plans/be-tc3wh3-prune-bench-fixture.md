# Plan — Prune scan bench fixture + NFR-02 guard test (be-tc3wh3)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-tc3wh3 (designer, 2026-05-15)
**Parent architecture:** be-5sn (prune feature)
**Dependency:** be-mrzg8o (CLOSED — bd prune reference protection available)

## Goal

Build test infrastructure to verify that `bd prune`'s reference scan completes in < 5s on a
10K-open-bead fixture (NFR-02). No changes to the prune implementation itself — only bench and
guard-test code.

## Decomposition decision: one builder bead

`buildPruneBenchFixture` helper + `BenchmarkPruneScan_10K` + `TestPruneScan_NFR02_Under5s`,
all in `cmd/bd/prune_bench_test.go` (or wherever prune tests live). Self-contained.

Builder bead: be-ss7x5n.

## Fixture parameters (from design §2.1)

| Parameter | Value |
|-----------|-------|
| Open beads | 10,000 |
| Body size per bead | ~5KB (description ~4.8KB + title ~80 chars) |
| Bead ID references per open bead | 0–3 random |
| Comments per bead | 0–2 (~1KB each) |
| Closed candidates | 1,000 |
| Referenced closed beads | ~100 (10% of candidates referenced by open beads) |

## Benchmark scope

Measures ONLY the reference scan (regex match over open bead bodies + comments), not full prune
command lifecycle. Range: from "candidates loaded" to "referenced set built".

## Acceptance (builder bead be-ss7x5n)

- `buildPruneBenchFixture(t, 10000, 1000)` completes in < 30s; skips via `testing.Short()`.
- `BenchmarkPruneScan_10K` compiles and produces allocs/op + ns/op.
- `TestPruneScan_NFR02_Under5s` passes locally with elapsed < 5s.
- Both tests skip cleanly under `go test -short`.
- NFR-02 documented in be-5sn's done-when criteria.

## No new build tag

Use `testing.Short()` only. Check existing test files for any `//go:build` slow-test convention
in the repo and follow it if present.

## Routing

- Builder bead: be-ss7x5n → `beads/builder`
- Design bead be-tc3wh3: CLOSED (PM work complete)

---
*— beads/pm, 2026-05-15*

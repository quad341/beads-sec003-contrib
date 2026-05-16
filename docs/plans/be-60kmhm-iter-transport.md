# Plan — bdd Daemon RPC Iterator Transport (be-60kmhm)

**PM:** beads/pm · **Date:** 2026-05-15
**Parent design:** be-60kmhm (designer, 2026-05-15)
**Parent architecture:** be-yinl4d → be-oyer9z (daemon framework)

## Goal

Add streaming iterator support to the bdd daemon's RPC surface. Phase 1
(be-oyer9z) had iterators fall back to slice mode via `BatchLimit`; this adds
the true pull-based streaming path. `bd list --json --limit 0` on a 100K-issue
rig must stay under 50 MB RSS CLI-side.

## Decomposition: 5 builder beads

```
be-zsr4id  Foundation: sentinel errors, config fields, EXTENDING.md
    ↓
be-e2pufb  iter_shared.go: session manager, shared types, server wiring
  (also depends on be-hpivhb — RPC package foundation)
    ↓
be-z09krv  bddgen extension: emit iter_types.go, iter_server.go, iter_client.go
  (also depends on be-jo8pxm — bddgen foundation)
    ↓ ↓
be-732qlr  DaemonStats iterator fields + bd daemon stats CLI UX
be-f49kcb  Unit tests (7) + parity integration tests (12)
```

## Bead summaries

| Bead | Title | Key files |
|------|-------|-----------|
| be-zsr4id | Foundation | storage.go, configfile.go, EXTENDING.md |
| be-e2pufb | Handwritten infra | internal/storage/rpc/iter_shared.go (NEW), server.go, client.go |
| be-z09krv | bddgen extension | cmd/bddgen/main.go → generates iter_types.go, iter_server.go, iter_client.go |
| be-732qlr | Stats + CLI UX | server.go DaemonStats, cmd/bd/daemon.go |
| be-f49kcb | Tests | iter_server_test.go (4), iter_client_test.go (3), parity/daemon_iter_test.go (12) |

## Key dependencies on Phase 1 beads

- be-hpivhb (RPC package types/server/client) must land before be-e2pufb extends its error tables
- be-jo8pxm (bddgen foundation) must land before be-z09krv extends it for Iter* methods

## Open questions for builder (from §14 of design)

1. **Prefetch** (`daemon_iter_prefetch`, default true): builder may omit in first pass if
   `rpc.Client.Go` complexity outweighs gain — create follow-up bead if deferred.
2. **Session ID format**: UUIDv4 (crypto/rand) is the default; base58-8-bytes is fine.
3. **Shutdown ordering**: `iterMgr.stop()` must fire AFTER `acceptLoop` exits in
   `daemon_child.go` — builder to confirm in be-e2pufb.
4. **DaemonIterIdleSeconds vs DaemonIdleSeconds**: independent mechanisms — document
   clearly in EXTENDING.md (done in be-zsr4id notes).

## Done-when (from design §13)

- [ ] `ErrTooManyIterators` and `ErrIterSessionNotFound` in `storage` + both RPC tables
- [ ] `DaemonIterBatch`, `DaemonIterMax`, `DaemonIterIdleSeconds` in configfile with defaults
- [ ] `bddgen` emits `iter_types.go`, `iter_server.go`, `iter_client.go` for all 10 Iter* methods
- [ ] `iterSessionManager`: thread-safe map, cap enforcement, idle reaper, drainAll on shutdown
- [ ] Each `rpcXxxIter` satisfies `storage.Iter[T]` and passes parity tests
- [ ] `rpcXxxIter.Next(ctx)` respects context cancellation
- [ ] Prefetch pipeline (or documented deferral)
- [ ] `errors.Is` works across RPC wire for both sentinels
- [ ] `IterClose` is idempotent, safe after daemon restart
- [ ] `bd daemon stats` shows iterator section with capacity coloring
- [ ] Bench: `bd list --json --limit 0` on 100K-issue rig ≤ 50 MB RSS CLI-side
- [ ] `TestDaemonIter_TooManyIterators`: fallback to slice path verified
- [ ] `TestDaemonIter_IdleReaper`: session reaped within idle window
- [ ] `go build ./...` clean; `golangci-lint` clean or matches baseline

## Routing

All 5 builder beads → `beads/builder`
Design bead be-60kmhm and PM bead be-g6dhzj: CLOSED (PM work complete)

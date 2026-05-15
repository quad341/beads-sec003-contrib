# Plan: `bdd` Daemon Mode Decomposition (be-oyer9z)

**PM:** beads/pm · **Date:** 2026-05-15
**Design bead:** be-oyer9z (design complete, from beads/designer)
**Parent architecture:** be-ef3q3u — bd cold-start RAM budget

## Summary

The designer completed the `bdd` daemon spec (Unix-socket RPC, `net/rpc + gob`, per-rig process holding open `storage.Storage`). This plan breaks that design into 8 sequentially-dependent builder beads.

**Goal:** Reduce bd CLI cold-start from 80 MB → <5 MB RPC per call.

## Open Questions (flagged to mayor)

| # | Question | Design recommendation | Blocking? |
|---|----------|-----------------------|-----------|
| Q1 | Env var name: `BEADS_DAEMON=off` vs `BEADS_DAEMON_MODE=off` | `BEADS_DAEMON_MODE=off` (matches metadata field name) | No — low churn either way |
| Q2 | PG beads: should `daemon_mode=auto` still default to off for PG? | Architect to confirm | No — builder adds code comment to revisit |
| Q3 | Iter* `BatchLimit`: server hard cap + truncation error vs in-process fallback | Server hard cap (forces callers to use in-process or wait for be-60kmhm) | Yes — builder needs decision before Iter* RPC impl |
| Q4 | `bdd.log` rotation: truncate on startup vs logrotate-style | Truncate on startup (`O_TRUNC`) | No |

## Bead Dependency Graph

```
A (foundation) ──┬──> B (bddgen) ──> C (rpc package)
                 │                        │
                 ├──> D (kill module)     │
                 │         │              │
                 │         └────────────────> E (daemon child + endpoint)
                 │                                   │
                 └───────────────────────────────────> F (factory integration)
                                                            │
                                                            ├──> G (daemon cmds + init + sql)
                                                            │
                                                            └──> H (tests)
```

## Bead Summaries

| Bead | Title | Key files | Deps |
|------|-------|-----------|------|
| be-fqjs3v | Foundation: configfile schema, pidfile extension, platform socket stubs | configfile.go, pidfile.go, daemon_socket_{unix,windows}.go, EXTENDING.md | — |
| be-jo8pxm | bddgen: go generate code generator for storage.Storage RPC surface | cmd/bddgen/main.go | be-fqjs3v |
| be-hpivhb | RPC package: types, server, client, error round-trip | internal/storage/rpc/ | be-jo8pxm |
| be-ht5qm4 | Daemon kill module | internal/daemon/kill.go | be-fqjs3v |
| be-t5hh3u | Daemon child + endpoint (lifecycle) | daemon_child.go, daemon_endpoint.go | be-fqjs3v, be-hpivhb, be-ht5qm4 |
| be-aksjau | Factory integration + --no-daemon flag | store_factory_{cgo,nocgo}.go, root cmd flag | be-fqjs3v, be-hpivhb, be-t5hh3u |
| be-2dv4s2 | `bd daemon` commands + RawDBAccessor handling + init reinit | daemon.go, sql.go, migration_validation.go, init.go | be-fqjs3v, be-hpivhb, be-ht5qm4, be-t5hh3u, be-aksjau |
| be-yu9ois | Tests: unit + parity (integration_daemon build tag) | daemon_child_test.go, client_test.go, daemon_parity_test.go | be-t5hh3u, be-aksjau, be-2dv4s2 |

## Done-When (from design §14)

- [ ] `bd daemon status / kill / stats` subcommands exist and output per design §5.3
- [ ] `metadata.json` accepts `daemon_mode`, `daemon_idle_seconds`, `daemon_max_lifetime_seconds` without breaking existing configs
- [ ] `openConfiguredStore` probes the daemon socket when `daemon_mode != "off"`
- [ ] `daemonClient` satisfies `storage.Storage`; `errors.Is(err, storage.ErrXxx)` works for all 5 sentinels
- [ ] `daemonClient` satisfies `storage.StoreLocator`; does NOT satisfy `storage.RawDBAccessor`
- [ ] `cmd/bd/sql.go` and `cmd/bd/doctor/migration_validation.go` handle `RawDBAccessor` miss gracefully
- [ ] `bd init --reinit` kills the daemon before writing metadata
- [ ] `bd daemon-child` acquires `bdd.lock`, exits code 75 on conflict
- [ ] Socket mode is `0600` (Unix only; Windows falls back to in-process)
- [ ] `bdd.pid` is valid JSON with `pid`, `socket`, `version`, `started_at` fields
- [ ] Smoke test (`TestDaemon_Smoke`) passes
- [ ] Concurrency test (`TestDaemon_ConcurrentCalls_NFR08`): 50 concurrent calls, p99 ≤ 2ms
- [ ] `go build ./...` clean; `golangci-lint run ./...` clean or matches LINTING.md baseline
- [ ] Telemetry deferred (documented in design)
- [ ] Windows: `sockPath()` returns `""` → factory always uses in-process

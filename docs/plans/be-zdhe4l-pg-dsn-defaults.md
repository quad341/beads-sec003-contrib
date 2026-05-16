# be-zdhe4l — Postgres DSN defaults + arbitrary host/db configuration

**Source:** be-zdhe4l (architect spec, 2026-05-09)
**Parent epic:** be-k4ur5v
**PM:** beads/pm (2026-05-16)
**Sibling:** be-pbnoaa (truthful backend reporting) — already decomposed and in progress

---

## Goal

Make `bd init --backend=postgres` work without a hand-written DSN when a local Postgres cluster is running, and add a complete `BEADS_POSTGRES_*` env-var override surface for runtime connection field overrides.

---

## Child beads

### be-5w6nt9 — DSN helpers (foundational, READY NOW)
`dsn.BuildFromFields` + `dsn.ApplyEnvOverrides` in `internal/storage/postgres/dsn`.
No blockers. The entry point for the whole feature.

### be-jnxg8d — Local PG discovery (blocks: be-5w6nt9)
`discoverLocalPostgres(clusterDir)` in `cmd/bd/init_postgres_discovery.go`.
Reads `postmaster.pid` preferred, `postgresql.conf` fallback. Read-only.

### be-fsobdu — bd init flags + flow (blocks: be-5w6nt9, be-jnxg8d)
New `--pg-host`, `--pg-port`, `--pg-user`, `--pg-database`, `--pg-sslmode`, `--pg-cluster-dir` flags.
Mutex with `--dsn`. Updated `runPostgresInit` with discovery → flags → DSN compose chain.
System-db rejection, non-loopback sslmode warning, `.beads/.discovery_log`.

### be-s67c8m — store_factory runtime wiring (blocks: be-5w6nt9)
Wire `dsn.ApplyEnvOverrides` before `dsn.Compose` in `store_factory.go` + `store_factory_nocgo.go`.
Enables BEADS_POSTGRES_* env-var overrides on every `bd` invocation without re-init.

---

## Dependency graph

```
be-5w6nt9 (DSN helpers)
├── be-jnxg8d (discovery)
│   └── be-fsobdu (bd init flow)
└── be-s67c8m (runtime wiring)
```

---

## Integration points

- **be-0w5z7u** (BackendInfo resolver, be-pbnoaa stream): after be-s67c8m lands, the resolver
  should also call `dsn.ApplyEnvOverrides` so `bd context` reflects runtime overrides. Note
  this in store_factory.go as a comment.
- **be-pbnoaa.4 doctor** (be-nzo2iu): `bd doctor` will consume the override-applied DSN and
  report `overrides_applied: [...]` in the JSON backend block.

---

## Out of scope (per be-zdhe4l §16)

- Local PG cluster lifecycle (start/stop/upgrade)
- `bd backend set` persistent override command
- IAM / cloud-managed PG auth (RDS IAM, Cloud SQL OAuth)
- Postgres TLS cert paths (sslrootcert, sslcert, sslkey)

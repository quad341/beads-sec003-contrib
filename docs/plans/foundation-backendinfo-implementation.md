# Foundation: BackendInfo Implementation Plan

This plan breaks down the designer's BackendInfo foundation work (be-brdzcs) into concrete implementation tasks for the builder agent.

## Overview

The foundation layer implements the core functionality for truthful backend reporting, including:
- BackendInfo struct and resolver
- Legacy Dolt state detection
- DSN rendering and parsing utilities

## Implementation Tasks

### 1. Implement BackendInfo struct and resolver
- **File**: `internal/configfile/backend_info.go`
- **Function**: `ResolveBackendInfo(beadsDir string) BackendInfo`
- **Description**: Reads metadata.json + env vars; returns resolved view. Does NOT open the database, does NOT ping anything. Pure resolution.
- **Acceptance Criteria**:
  - BackendInfo struct matches the JSON shape in the architecture doc
  - All new functions have godoc
  - 100% line coverage on the resolver
  - `go test ./internal/configfile/...` passes

### 2. Implement legacy Dolt state detection
- **File**: `internal/configfile/legacy.go`
- **Function**: `DetectLegacyDoltState(beadsDir, cfg) (legacyDir string, legacyFields []string)`
- **Description**: Detects `.beads/dolt` while backend=postgres, populated dolt_* fields while backend=postgres.
- **Acceptance Criteria**:
  - 100% line coverage on the legacy detection
  - All new functions have godoc
  - `go test ./internal/configfile/...` passes

### 3. Implement DSN render and parser functions
- **File**: `internal/storage/postgres/dsn/render.go`
- **Functions**: `RenderRedacted(dsn string) (string, error)` and `ParseConnectionTarget(dsn string) (host, port, db, user, sslmode, error)`
- **Description**: Both go through pgconn.ParseConfig + ConfigToConnString; defence-in-depth strip even if input had a password.
- **Acceptance Criteria**:
  - 100% line coverage on the renderer and parser
  - All new functions have godoc
  - `go test ./internal/storage/postgres/dsn/...` passes
  - TestRenderRedacted_NeverLeaksPassword — inputs include 's3cretP@ss' password in DSN, hand-edited DSN, double-encoded password, malformed DSN; assert output never contains the password substring

## Dependencies

All tasks are independent and can be worked on in parallel.
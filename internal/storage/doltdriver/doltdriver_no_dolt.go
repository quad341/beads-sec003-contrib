//go:build no_dolt

// Package doltdriver — no_dolt stub variant.
//
// Under -tags no_dolt the bd binary excludes the Dolt and embedded-Dolt
// engines entirely. This file registers BackendDolt with a factory that
// returns ErrNoDoltSupport so users with metadata.json backend=dolt see a
// clean install-guidance error instead of "unknown backend."
//
// This file MUST NOT import internal/storage/dolt, internal/storage/embeddeddolt,
// or any github.com/dolthub/* package — that's the load-bearing constraint that
// keeps the no_dolt binary small.
package doltdriver

import (
	"context"
	"errors"
	"strings"

	"github.com/steveyegge/beads/internal/storage"
)

// ErrNoDoltSupport is returned by the registered factory when bd was built
// with -tags no_dolt and the user's metadata.json selects Dolt.
var ErrNoDoltSupport = errors.New(NoDoltSupportErrMsg)

// NoDoltSupportErrMsg is the user-facing message. Adapted from
// NoCGOEmbeddedErrMsg in the !cgo variant.
const NoDoltSupportErrMsg = `this build of bd does not include the Dolt backend

bd was built with -tags no_dolt, which produces a smaller binary that
supports only the Postgres backend. Your metadata.json selects backend=dolt.

To use this rig:
  - Switch to a default bd build (which includes both Dolt and Postgres)
    by reinstalling: go install github.com/steveyegge/beads/cmd/bd@latest
  - Or migrate the rig to Postgres: bd init --backend=postgres --dsn=...`

func init() {
	storage.RegisterDriver(storage.BackendDolt, openDoltStoreNoSupport)
}

func openDoltStoreNoSupport(_ context.Context, _ storage.ConnectionConfig) (storage.Storage, error) {
	return nil, ErrNoDoltSupport
}

// SanitizeDBName mirrors the same-named helper in the !no_dolt variants so
// cmd/bd compiles without a build-tag fork at the call site. Under no_dolt
// the function is unreachable in practice — every Dolt-backend open errors
// out before any caller reaches name normalization — but it must be defined.
func SanitizeDBName(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}

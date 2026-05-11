//go:build no_dolt

// Package embeddeddolt — no_dolt stub variant.
//
// Under -tags no_dolt the bd binary excludes the embedded Dolt engine
// entirely. This file provides the same Open/OpenSQL/EmbeddedDoltStore
// surface as the !cgo stub variant so cmd/bd compiles unchanged. All
// constructors return ErrNoDoltSupport; the result type's only consumer
// is store_factory_dolt_cgo.go (which is excluded under no_dolt).
//
// This file MUST NOT import any github.com/dolthub/* package.
package embeddeddolt

import (
	"context"
	"database/sql"
	"errors"

	"github.com/steveyegge/beads/internal/storage"
)

// EmbeddedDoltStore is a placeholder under no_dolt. The constructor
// returns (nil, ErrNoDoltSupport), so no caller dereferences this value.
//
// Embedding storage.DoltStorage compile-satisfies the full Dolt-backed
// store capability set so cmd/bd can pass *EmbeddedDoltStore to functions
// that expect any of the storage interfaces without us enumerating every
// method.
type EmbeddedDoltStore struct {
	storage.DoltStorage
}

// Compile-time assertion that *EmbeddedDoltStore satisfies storage.DoltStorage.
var _ storage.DoltStorage = (*EmbeddedDoltStore)(nil)

// ErrNoDoltSupport is returned by every constructor under no_dolt.
var ErrNoDoltSupport = errors.New("embedded Dolt is not compiled in (built with -tags no_dolt)")

// Open returns ErrNoDoltSupport — the embedded Dolt engine is excluded.
func Open(_ context.Context, _, _, _ string) (*EmbeddedDoltStore, error) {
	return nil, ErrNoDoltSupport
}

// OpenSQL returns ErrNoDoltSupport — the embedded Dolt SQL surface is excluded.
func OpenSQL(_ context.Context, _, _, _ string) (*sql.DB, func() error, error) {
	return nil, nil, ErrNoDoltSupport
}

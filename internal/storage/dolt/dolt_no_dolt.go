//go:build no_dolt

// Package dolt — no_dolt stub variant.
//
// Under -tags no_dolt the bd binary excludes the Dolt storage backend
// entirely. This file provides the public surface (Config, DoltStore,
// constructors) that cmd/bd, version_tracking, ado, etc. reference
// unconditionally, so the binary compiles without any github.com/dolthub/*
// dependency.
//
// All constructors return (nil, ErrNoDoltSupport). cmd/bd is expected to
// surface that error via openConfiguredStore (in store_factory_no_dolt.go),
// which intercepts backend=dolt before any of these are reached. The
// methods on DoltStore exist for compile-time satisfaction; they panic
// because the constructor never returns a non-nil receiver under no_dolt.
//
// This file MUST NOT import any github.com/dolthub/* package — that's
// the load-bearing constraint that keeps the no_dolt binary small.
package dolt

import (
	"context"
	"database/sql"
	"errors"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// ErrNoDoltSupport is returned by every constructor under no_dolt.
var ErrNoDoltSupport = errors.New("Dolt backend is not compiled in (built with -tags no_dolt)")

// Config mirrors the !no_dolt struct so callers compile unchanged. Only
// the field set is preserved; under no_dolt the values are inert.
type Config struct {
	Path            string
	BeadsDir        string
	CommitterName   string
	CommitterEmail  string
	Remote          string
	Database        string
	ReadOnly        bool
	ServerSocket    string
	ServerHost      string
	ServerPort      int
	ServerUser      string
	ServerPassword  string
	ServerTLS       bool
	RemoteUser      string
	RemotePassword  string
	SyncRemote      string
	CreateIfMissing bool
	ServerMode      bool
	AutoStart       bool
	MaxOpenConns    int
	MaxIdleConns    int
}

// DoltStore is a placeholder under no_dolt. The constructor returns
// (nil, ErrNoDoltSupport), so no caller dereferences this value.
//
// We embed storage.DoltStorage (the full Dolt capability interface)
// so the type compile-satisfies every method consumers might call,
// including GetMetadata/SetMetadata, version-control, federation, etc.
// Under no_dolt the embedded value is always nil and never called —
// callers bail on the constructor's error return.
type DoltStore struct {
	storage.DoltStorage
}

// New returns ErrNoDoltSupport — the Dolt backend is excluded.
func New(_ context.Context, _ *Config) (*DoltStore, error) {
	return nil, ErrNoDoltSupport
}

// NewFromConfig returns ErrNoDoltSupport.
func NewFromConfig(_ context.Context, _ string) (*DoltStore, error) {
	return nil, ErrNoDoltSupport
}

// NewFromConfigWithOptions returns ErrNoDoltSupport.
func NewFromConfigWithOptions(_ context.Context, _ string, _ *Config) (*DoltStore, error) {
	return nil, ErrNoDoltSupport
}

// NewFromConfigWithCLIOptions returns ErrNoDoltSupport.
func NewFromConfigWithCLIOptions(_ context.Context, _ string, _ *Config) (*DoltStore, error) {
	return nil, ErrNoDoltSupport
}

// GetBackendFromConfig returns the empty string under no_dolt — there is
// no Dolt-side metadata.json to read. The Postgres backend uses configfile
// directly, not this helper.
func GetBackendFromConfig(_ string) string { return "" }

// ValidateDatabaseName returns ErrNoDoltSupport for any name. Callers in
// init.go path through this only when --backend=dolt was selected; that
// path is rejected earlier by store_factory_no_dolt.go, so this is
// unreachable in practice.
func ValidateDatabaseName(_ string) error { return ErrNoDoltSupport }

// CleanStaleCircuitBreakerFiles is a no-op under no_dolt — no Dolt
// circuit breakers exist.
func CleanStaleCircuitBreakerFiles() {}

// DefaultInfraTypes mirrors the !no_dolt list. Used by export_auto.go
// when generating the default infra-type set; the value is independent
// of the Dolt backend, so we return the same defaults.
func DefaultInfraTypes() []string {
	return []string{"agent", "rig", "role", "message"}
}

// HasBackupFiles always reports false under no_dolt — no Dolt backups
// can exist when the backend isn't compiled in.
func HasBackupFiles(_ string) bool { return false }

// ApplyCLIAutoStart is a no-op under no_dolt — there's no Dolt CLI to
// auto-start.
func ApplyCLIAutoStart(_ string, _ *Config) {}

// BootstrapFromRemoteWithDB returns ErrNoDoltSupport — Dolt remotes are
// not reachable under no_dolt.
func BootstrapFromRemoteWithDB(_ context.Context, _, _, _ string) (bool, error) {
	return false, ErrNoDoltSupport
}

// CreateIgnoredTables is a no-op under no_dolt — only test_helpers_test.go
// references it, and that file is excluded under -tags no_dolt via the
// test surface audit.
func CreateIgnoredTables(_ *sql.DB) error { return ErrNoDoltSupport }

// DB returns nil — only test helpers (excluded under no_dolt) reference this.
// Defined explicitly because *sql.DB access is on RawDBAccessor (a
// separate interface from DoltStorage), not part of the embedding.
func (*DoltStore) DB() *sql.DB { return nil }

// UnderlyingDB returns nil. Same rationale as DB.
func (*DoltStore) UnderlyingDB() *sql.DB { return nil }

// Compile-time assertion that DoltStore satisfies the full DoltStorage
// interface via embedding. Calls on the embedded nil interface will
// panic at runtime, which is the desired behavior — no caller should
// reach a method on a store whose constructor returned (nil, ErrNoDoltSupport).
var _ storage.DoltStorage = (*DoltStore)(nil)

// Unused-imports defensive: keep types import live so the stub's import
// graph aligns with the !no_dolt variant in case future stub additions
// need to return a types.* value.
var _ = types.StatusOpen

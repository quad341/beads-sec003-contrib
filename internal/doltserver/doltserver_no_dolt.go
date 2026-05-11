//go:build no_dolt

// Package doltserver — no_dolt stub variant.
//
// Under -tags no_dolt the bd binary excludes the Dolt server lifecycle
// orchestrator entirely. This file provides minimal stubs of the public
// surface that cmd/bd references unconditionally (status output, port
// resolution, server-mode probes), so the binary compiles without any
// github.com/dolthub/* dependency.
//
// Functions whose semantics require a live Dolt server return a benign
// zero value or ErrNoDoltSupport. They are unreachable in practice
// because cmd/bd's openConfiguredStore (in store_factory_no_dolt.go)
// rejects backend=dolt before any of these are exercised.
//
// This file MUST NOT import any github.com/dolthub/* package — that's
// the load-bearing constraint that keeps the no_dolt binary small.
package doltserver

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
)

// ErrNoDoltSupport mirrors doltdriver.ErrNoDoltSupport but lives here so
// doltserver callers don't need to import doltdriver.
var ErrNoDoltSupport = errors.New("dolt server support is not compiled in (built with -tags no_dolt)")

// ErrServerNotRunning mirrors the error from the !no_dolt variant.
var ErrServerNotRunning = errors.New("dolt server is not running")

// State holds runtime information about a managed server.
type State struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid"`
	Port    int    `json:"port"`
	DataDir string `json:"data_dir"`
}

// Config holds the server configuration.
type Config struct {
	BeadsDir string
	Port     int
	Host     string
	Mode     ServerMode
}

// ServerMode describes who owns the server lifecycle. Under no_dolt the
// only meaningful value is ServerModeOwned (zero value); any caller that
// branches on this should fail closed.
type ServerMode int

const (
	ServerModeOwned ServerMode = iota
	ServerModeExternal
	ServerModeEmbedded
)

func (m ServerMode) String() string {
	switch m {
	case ServerModeOwned:
		return "owned"
	case ServerModeExternal:
		return "external"
	case ServerModeEmbedded:
		return "embedded"
	default:
		return "unknown"
	}
}

const (
	PIDFileName  = "dolt-server.pid"
	PortFileName = "dolt-server.port"
)

const DefaultSharedServerPort = 3308

const GlobalDatabaseName = "beads_global"

const GlobalIssuePrefix = "global"

const GlobalProjectID = "00000000-0000-0000-0000-000000000000"

// IsSharedServerMode is always false under no_dolt — there is no Dolt
// server to share.
func IsSharedServerMode() bool { return false }

// IsAutoStartDisabled is always true under no_dolt — there's no server
// to auto-start.
func IsAutoStartDisabled() bool { return true }

// SharedServerDir returns ErrNoDoltSupport — no shared Dolt server.
func SharedServerDir() (string, error) { return "", ErrNoDoltSupport }

// SharedDoltDir returns ErrNoDoltSupport — no shared Dolt data dir.
func SharedDoltDir() (string, error) { return "", ErrNoDoltSupport }

// ResolveServerDir mirrors the !no_dolt computation that doesn't depend
// on Dolt itself; returned path is purely for display/debug under no_dolt
// since no Dolt server is started.
func ResolveServerDir(beadsDir string) string { return beadsDir }

// ResolveDoltDir returns the conventional .beads/dolt path. No directory
// is ever populated under no_dolt; the value exists so display-side code
// (e.g., bd doctor) renders the same path the !no_dolt build would.
func ResolveDoltDir(beadsDir string) string {
	return filepath.Join(beadsDir, "dolt")
}

// EnsurePortFile is a no-op under no_dolt.
func EnsurePortFile(_ string, _ int) error { return nil }

// ReadPortFile returns 0 — there is no port file under no_dolt.
func ReadPortFile(_ string) int { return 0 }

// DefaultConfig returns a Config with BeadsDir filled in. The server is
// never started, so the other fields are placeholders.
func DefaultConfig(beadsDir string) *Config {
	return &Config{BeadsDir: beadsDir}
}

// IsRunning always reports the server as not running under no_dolt.
func IsRunning(_ string) (*State, error) {
	return &State{Running: false}, nil
}

// EnsureRunningDetailed cannot start a server under no_dolt; returns
// ErrNoDoltSupport.
func EnsureRunningDetailed(_ string) (port int, startedByUs bool, err error) {
	return 0, false, ErrNoDoltSupport
}

// Start cannot start a Dolt server under no_dolt.
func Start(_ string) (*State, error) { return nil, ErrNoDoltSupport }

// EnsureGlobalDatabase has no effect under no_dolt.
func EnsureGlobalDatabase(_ string, _ int, _, _ string) error { return ErrNoDoltSupport }

// Stop is a no-op under no_dolt — there is no server to stop.
func Stop(_ string) error { return nil }

// StopWithForce is a no-op under no_dolt.
func StopWithForce(_ string, _ bool) error { return nil }

// LogPath returns the conventional log path; the file is never written.
func LogPath(beadsDir string) string {
	return filepath.Join(beadsDir, "dolt-server.log")
}

// KillStaleServers is a no-op — there are no servers to kill.
func KillStaleServers(_ string) ([]int, error) { return nil, nil }

// RecoverPreV56DoltDir is a no-op — there's no Dolt directory to recover.
func RecoverPreV56DoltDir(_ string) (bool, error) { return false, nil }

// IsPreV56DoltDir always reports false under no_dolt.
func IsPreV56DoltDir(_ string) bool { return false }

// ResolveServerMode always returns ServerModeOwned — under no_dolt the
// caller should be reaching this only on the !backend=dolt path. The
// value chosen is the most benign default.
func ResolveServerMode(_ string) ServerMode { return ServerModeOwned }

// IgnoreNotRunning lets callers swallow the "not running" sentinel
// without importing the errors package. Under no_dolt every status check
// reports not-running, so this is exercised more often than under !no_dolt.
func IgnoreNotRunning(err error) error {
	if errors.Is(err, ErrServerNotRunning) {
		return nil
	}
	return err
}

// connectStub is unused under no_dolt — kept here so callers that take a
// *sql.DB compile. It returns a closed sentinel; callers must handle the
// error from the upstream Open/Connect path.
var _ = func(_ context.Context) (*sql.DB, error) { return nil, ErrNoDoltSupport }

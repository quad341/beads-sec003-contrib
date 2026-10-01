package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/dbproxy/proxy"
	"github.com/steveyegge/beads/internal/storage/dbproxy/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDatabaseServer_BackendExternal(t *testing.T) {
	t.Run("valid tcp config builds an ExternalDoltServer", func(t *testing.T) {
		srv, err := newDatabaseServer(
			proxy.BackendExternal,
			"", "", "", "", "",
			configfile.ExternalDoltConfig{Host: "db.internal", Port: 3306},
		)
		require.NoError(t, err)
		require.NotNil(t, srv)
		_, ok := srv.(*server.ExternalDoltServer)
		assert.True(t, ok, "expected *server.ExternalDoltServer, got %T", srv)
	})

	t.Run("invalid config bubbles validation error", func(t *testing.T) {
		_, err := newDatabaseServer(
			proxy.BackendExternal,
			"", "", "", "", "",
			configfile.ExternalDoltConfig{},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ExternalDoltConfig")
	})

	t.Run("unix socket config builds an ExternalDoltServer", func(t *testing.T) {
		// ExternalDoltConfig.Validate() requires an absolute Socket via filepath.IsAbs,
		// which is platform-dependent: /var/run/dolt.sock is NOT absolute on Windows
		// (no volume). Use a platform-absolute socket path so the same construction
		// path is exercised everywhere.
		socket := "/var/run/dolt.sock"
		if runtime.GOOS == "windows" {
			socket = filepath.Join(t.TempDir(), "dolt.sock")
		}
		srv, err := newDatabaseServer(
			proxy.BackendExternal,
			"", "", "", "", "",
			configfile.ExternalDoltConfig{Socket: socket},
		)
		require.NoError(t, err)
		require.NotNil(t, srv)
		_, ok := srv.(*server.ExternalDoltServer)
		assert.True(t, ok)
	})
}

func TestNewDatabaseServer_BackendLocalSharedServerStillStubbed(t *testing.T) {
	_, err := newDatabaseServer(
		proxy.BackendLocalSharedServer,
		"/tmp/root", "/tmp/cfg", "/tmp/log", "/usr/bin/dolt", "",
		configfile.ExternalDoltConfig{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestNewDatabaseServer_UnknownBackendRejected(t *testing.T) {
	_, err := newDatabaseServer(
		proxy.Backend("bogus"),
		"", "", "", "", "",
		configfile.ExternalDoltConfig{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown backend")
}

func TestDbProxyChildRegistersExternalFlags(t *testing.T) {
	cases := []struct {
		name        string
		defaultText string
	}{
		{"external-host", ""},
		{"external-port", "0"},
		{"external-socket-path", ""},
		{"external-keep-alive", "0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := dbProxyChildCmd.Flags().Lookup(tc.name)
			require.NotNil(t, f, "db-proxy-child does not register --%s", tc.name)
			assert.Equal(t, tc.defaultText, f.DefValue, "--%s default", tc.name)
		})
	}
}

// TestDbProxyChildIdleTimeoutHelpNamesEphemeralRoot guards be-7272o's
// acceptance criteria: the hidden db-proxy-child command's own --idle-timeout
// flag must not describe "0 or negative = never shut down" in isolation now
// that BEADS_EPHEMERAL_ROOT exists -- a reader would wrongly conclude 0 is
// always safe to pass here. It must name the env var, mirroring init.go's
// --proxied-server-idle-timeout help (gastownhall/beads#6755, be-466tg's
// response commit: TestInitIdleTimeoutHelpNamesEphemeralRoot).
func TestDbProxyChildIdleTimeoutHelpNamesEphemeralRoot(t *testing.T) {
	f := dbProxyChildCmd.Flags().Lookup("idle-timeout")
	require.NotNil(t, f, "db-proxy-child does not register --idle-timeout")
	assert.Contains(t, f.Usage, "BEADS_EPHEMERAL_ROOT=1",
		"db-proxy-child --idle-timeout help does not name BEADS_EPHEMERAL_ROOT=1")
}

// TestDbProxyChildIdleTimeoutHelpHasNoForcedWindow guards against
// reintroducing the 45s forced-default-window design be-hjyio's 2026-09-26
// re-scoping ruling dropped (be-djq0v notes): the corrected
// BEADS_EPHEMERAL_ROOT semantics are fail-fast-only on an explicit 0 or
// negative value, never a silent override of an omitted or explicit
// positive idle-timeout.
func TestDbProxyChildIdleTimeoutHelpHasNoForcedWindow(t *testing.T) {
	f := dbProxyChildCmd.Flags().Lookup("idle-timeout")
	require.NotNil(t, f, "db-proxy-child does not register --idle-timeout")
	assert.NotContains(t, f.Usage, "45s",
		"db-proxy-child --idle-timeout help mentions a 45s forced window, which be-hjyio's re-scoping dropped")
}

// TestDbProxyChildIdleTimeoutHelpKeepsOmittedDefaultAndNever guards be-7272o's
// second acceptance criterion. gastownhall/beads#6753 (be-ijli2/be-pc60t)
// rewrites this same flag's help line: its contract is that the parent resolves
// an omitted value to the default (configfile.DefaultProxyIdleTimeout) before
// this flag is ever set, and that 0 means never. Ours is BEADS_EPHEMERAL_ROOT=1
// (TestDbProxyChildIdleTimeoutHelpNamesEphemeralRoot). The two PRs conflict on
// that one line, so whichever merges second must keep both wordings; pinning
// the default and "never" here, next to the ephemeral-root pin, makes a
// conflict resolution that drops either sentence fail a test instead of
// silently clobbering the other PR's wording.
func TestDbProxyChildIdleTimeoutHelpKeepsOmittedDefaultAndNever(t *testing.T) {
	f := dbProxyChildCmd.Flags().Lookup("idle-timeout")
	require.NotNil(t, f, "db-proxy-child does not register --idle-timeout")
	assert.Contains(t, f.Usage, configfile.DefaultProxyIdleTimeout.String(),
		"db-proxy-child --idle-timeout help does not state the default an omitted value resolves to")
	assert.Contains(t, f.Usage, "never",
		"db-proxy-child --idle-timeout help does not say 0 means never")
}

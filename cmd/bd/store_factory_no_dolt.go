//go:build no_dolt

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/db/util"
	"github.com/steveyegge/beads/internal/storage/dolt"
	_ "github.com/steveyegge/beads/internal/storage/doltdriver" // self-registers BackendDolt with the no_dolt stub factory
	"github.com/steveyegge/beads/internal/storage/postgres/dsn"
)

// NoDoltBinaryMsg is the user-facing error when a rig configured for the
// Dolt backend is opened by a no_dolt build. cmd/bd surfaces this from
// openConfiguredStore so the user gets actionable install-guidance instead
// of the registry's "unknown backend" sentinel.
const NoDoltBinaryMsg = `this rig uses backend=dolt, but bd was built with -tags no_dolt
which excludes the Dolt backend.

To use this rig:
  - Reinstall a default bd build that includes Dolt:
      go install github.com/steveyegge/beads/cmd/bd@latest
  - Or migrate to Postgres: bd init --backend=postgres --dsn=...`

// isEmbeddedMode returns false under no_dolt — embedded Dolt is excluded.
func isEmbeddedMode() bool { return false }

// newDoltStore returns NoDoltBinaryMsg. Bootstrap call sites still
// reference this function under shared file-level entry points; under
// no_dolt those are guarded by isEmbeddedMode/configfile checks that
// would never reach this in practice.
func newDoltStore(_ context.Context, _ *dolt.Config) (storage.Storage, error) {
	return nil, fmt.Errorf("%s", NoDoltBinaryMsg)
}

// acquireEmbeddedLock is a no-op under no_dolt — there is no embedded
// Dolt directory to protect.
func acquireEmbeddedLock(_ string, _ bool) (util.Unlocker, error) {
	return util.NoopLock{}, nil
}

func newStoreFromConfig(ctx context.Context, beadsDir string) (storage.Storage, error) {
	return openConfiguredStore(ctx, beadsDir, false)
}

func newReadOnlyStoreFromConfig(ctx context.Context, beadsDir string) (storage.Storage, error) {
	return openConfiguredStore(ctx, beadsDir, true)
}

// newDoltStoreFromConfig path-throughs to the registry, which under
// no_dolt returns ErrNoDoltSupport via the stub factory registered in
// internal/storage/doltdriver/doltdriver_no_dolt.go. Callers that pin
// the Dolt backend (federation, dolt-only subcommands) are themselves
// excluded under no_dolt; this stub exists only so shared bootstrap
// paths compile.
func newDoltStoreFromConfig(ctx context.Context, beadsDir string) (storage.Storage, error) {
	return storage.Open(ctx, storage.BackendDolt, storage.ConnectionConfig{BeadsDir: beadsDir})
}

func openConfiguredStore(ctx context.Context, beadsDir string, readOnly bool) (storage.Storage, error) {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return nil, err
	}

	if cfg != nil && cfg.GetBackend() == configfile.BackendDolt {
		// FR-03: clear install-guidance error rather than letting the
		// registry's stub factory error message stand alone. The
		// configfile path tells the user the rig is configured for Dolt,
		// which is more actionable than the registry's "unknown backend."
		return nil, fmt.Errorf("%s", NoDoltBinaryMsg)
	}

	if cfg == nil || cfg.GetBackend() != configfile.BackendPostgres {
		return nil, fmt.Errorf("metadata.json: backend=%q is not supported in this build (only postgres is available under -tags no_dolt)",
			backendName(cfg))
	}
	if cfg.PostgresDSN == "" {
		return nil, fmt.Errorf("metadata.json: backend=postgres but postgres_dsn is empty (run `bd init --backend=postgres --dsn=...`)")
	}

	connCfg := storage.ConnectionConfig{
		BeadsDir: beadsDir,
		ReadOnly: readOnly,
		DSN:      dsn.Compose(cfg.PostgresDSN, os.Getenv("BEADS_POSTGRES_PASSWORD")),
	}
	return storage.Open(ctx, storage.BackendPostgres, connCfg)
}

func backendName(cfg *configfile.Config) string {
	if cfg == nil {
		return ""
	}
	return string(cfg.GetBackend())
}

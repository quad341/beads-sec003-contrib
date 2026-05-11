//go:build no_dolt

package main

import (
	"context"
	"fmt"

	"github.com/steveyegge/beads/internal/storage"
)

// runBackupRestore is unreachable under no_dolt in practice — bootstrap's
// executeBackupRestoreAction calls newDoltStore first, which fails with
// NoDoltBinaryMsg in this build. The stub exists so the shared bootstrap
// file compiles, and returns a deterministic error if the unreachable path
// is somehow taken.
func runBackupRestore(_ context.Context, _ storage.Storage, _ string, _ bool) error {
	return fmt.Errorf("backup restore unavailable: bd was built with -tags no_dolt (Dolt-native backup is excluded)")
}

// Package versionedhistory resolves and applies dual-write issue-version
// history activation to one storage instance.
//
// It is a package rather than a helper inside cmd/bd for the same reason as
// internal/eventsjournal: bd doctor's repair handlers live in their own
// package and cannot import package main, and a repair that mutates issues
// needs the same activation as the command surface.
//
// Unlike the events journal, activation here reads the STORE's own config
// (GetConfig), not a workspace's config.yaml or an environment variable —
// versioned-history.enabled is a per-rig database setting, set with `bd
// config set` and read back at store-construction time, so there is no
// env/file precedence ladder to resolve.
//
// Activation is applied by the FACTORIES that construct a store, mirroring
// internal/eventsjournal's own placement; see cmd/bd/events_journal.go for
// the wiring shape this follows.
package versionedhistory

import (
	"context"
	"strconv"
	"strings"

	"github.com/steveyegge/beads/internal/storage"
)

// ConfigKey is the store-level setting that turns dual-write issue-version
// history on for that store instance only. Documented in
// docs/reference/configuration.md's Project-Level Settings (Database) table;
// that table and this constant must never drift from each other.
const ConfigKey = "versioned-history.enabled"

// Apply binds a resolved activation to one storage instance.
//
// Unlike internal/eventsjournal's Apply, this never fails: storage.go's own
// doc comment for VersionedHistoryConfigurer says callers type-assert and "a
// store that does not implement it simply cannot record version history" —
// there is no silently-broken guarantee to protect here the way there is for
// the events journal, so a nil configurer is a plain no-op.
func Apply(configurer storage.VersionedHistoryConfigurer, enabled bool) {
	if configurer == nil {
		return
	}
	configurer.SetVersionedHistoryEnabled(enabled)
}

// ActivateStore reads ConfigKey from a freshly opened store and applies it.
// It is written to be used as a deferred rewrite of a factory's named
// results, the same shape as internal/eventsjournal.ActivateStore:
//
//	func newX(...) (s storage.DoltStorage, err error) {
//	    defer func() { s, err = versionedhistory.ActivateStore(ctx, s, err) }()
//	    ... open and return ...
//	}
//
// A failed open passes through untouched. A store that cannot report its own
// config (GetConfig error) or that cannot version at all is left at the
// default, disabled — fail OPEN, not closed, per Apply's own doc comment.
func ActivateStore(ctx context.Context, s storage.DoltStorage, err error) (storage.DoltStorage, error) {
	if err != nil || s == nil {
		return s, err
	}
	raw, getErr := s.GetConfig(ctx, ConfigKey)
	enabled := false
	if getErr == nil {
		enabled, _ = strconv.ParseBool(strings.TrimSpace(raw))
	}
	configurer, _ := storage.UnwrapStore(s).(storage.VersionedHistoryConfigurer)
	Apply(configurer, enabled)
	return s, nil
}

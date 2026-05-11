//go:build integration_pg

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
)

// TestSetConfigDispatchTypesCustom verifies the store-level SetConfig router:
// types.custom writes the config row AND populates custom_types atomically,
// and a re-SetConfig with a smaller value fully replaces the table
// (DELETE+INSERT semantics, mirroring dolt).
func TestSetConfigDispatchTypesCustom(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	if err := store.SetConfig(ctx, "types.custom", `["a","b"]`); err != nil {
		t.Fatalf("set types.custom: %v", err)
	}
	got, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get custom types: %v", err)
	}
	if !equalStrings(got, []string{"a", "b"}) {
		t.Errorf("after first set: got %v, want [a b]", got)
	}

	if err := store.SetConfig(ctx, "types.custom", `["a"]`); err != nil {
		t.Fatalf("re-set types.custom: %v", err)
	}
	got, err = store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get custom types after re-set: %v", err)
	}
	if !equalStrings(got, []string{"a"}) {
		t.Errorf("after re-set: got %v, want [a]", got)
	}

	value, err := store.GetConfig(ctx, "types.custom")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if value != `["a"]` {
		t.Errorf("config row: got %q, want %q", value, `["a"]`)
	}
}

// TestSetConfigDispatchStatusCustom verifies the store-level SetConfig router
// for status.custom: the category-aware entries land in custom_statuses with
// the correct Category column.
func TestSetConfigDispatchStatusCustom(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	if err := store.SetConfig(ctx, "status.custom", "reviewing:active,shipped:done"); err != nil {
		t.Fatalf("set status.custom: %v", err)
	}
	detailed, err := store.GetCustomStatusesDetailed(ctx)
	if err != nil {
		t.Fatalf("get custom statuses: %v", err)
	}
	if len(detailed) != 2 {
		t.Fatalf("want 2 rows, got %d: %v", len(detailed), detailed)
	}
	byName := map[string]string{}
	for _, s := range detailed {
		byName[s.Name] = string(s.Category)
	}
	if byName["reviewing"] != "active" {
		t.Errorf("reviewing category: got %q, want %q", byName["reviewing"], "active")
	}
	if byName["shipped"] != "done" {
		t.Errorf("shipped category: got %q, want %q", byName["shipped"], "done")
	}
}

// TestSetConfigDispatchFastPathUnchanged verifies non-synced keys still take
// the fast path: setting issue_prefix must not touch custom_types and must
// not require a transaction.
func TestSetConfigDispatchFastPathUnchanged(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	// Seed custom_types so we can detect whether the fast path touches it.
	if err := store.SetConfig(ctx, "types.custom", `["preexisting"]`); err != nil {
		t.Fatalf("seed types.custom: %v", err)
	}
	if err := store.SetConfig(ctx, "issue_prefix", "bd"); err != nil {
		t.Fatalf("set issue_prefix: %v", err)
	}
	got, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get custom types: %v", err)
	}
	if !equalStrings(got, []string{"preexisting"}) {
		t.Errorf("fast-path SetConfig touched custom_types: got %v, want [preexisting]", got)
	}

	// Sanity-check that issue_prefix actually landed.
	value, err := store.GetConfig(ctx, "issue_prefix")
	if err != nil {
		t.Fatalf("get issue_prefix: %v", err)
	}
	if value != "bd" {
		t.Errorf("issue_prefix: got %q, want %q", value, "bd")
	}
}

// TestSetConfigDispatchRollbackInTx verifies that when a caller-supplied tx
// (via RunInTransaction) rolls back, both the config write AND the
// custom_types sync are undone — i.e. the sync rides on the caller's tx, not
// a nested one.
func TestSetConfigDispatchRollbackInTx(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	sentinel := errors.New("force rollback")
	err = store.RunInTransaction(ctx, "", func(tx storage.Transaction) error {
		if err := tx.SetConfig(ctx, "types.custom", `["should_not_persist"]`); err != nil {
			t.Fatalf("tx.SetConfig: %v", err)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunInTransaction: got %v, want sentinel error", err)
	}

	// Config row must NOT be present.
	value, err := store.GetConfig(ctx, "types.custom")
	if err != nil {
		t.Fatalf("get config after rollback: %v", err)
	}
	if value != "" {
		t.Errorf("config persisted after rollback: %q", value)
	}

	// Mirror table must be empty.
	got, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get custom types after rollback: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("custom_types persisted after rollback: %v", got)
	}
}

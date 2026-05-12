//go:build integration_pg

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
)

// resetForMigrationV3Test prepares the schema for re-running v3: it removes
// the bookkeeping row, clears the mirror tables, and seeds the config rows
// the caller provides. Pass empty values to skip seeding for a key.
func resetForMigrationV3Test(t *testing.T, store *PostgresStore, typesValue, statusValue string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete every migration row at or above v3 so the runner re-applies v3
	// on the next pass. The loop in runMigrations skips any migration whose
	// version <= current, so leaving v4+ bookkeeping in place would prevent
	// v3 from re-running. v4 is idempotent (DROP/ADD CONSTRAINT IF EXISTS),
	// so re-applying it alongside v3 is harmless.
	if _, err := store.pool.Exec(ctx, "DELETE FROM bd_schema_migrations WHERE version >= 3"); err != nil {
		t.Fatalf("delete migrations row: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "DELETE FROM custom_types"); err != nil {
		t.Fatalf("clear custom_types: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "DELETE FROM custom_statuses"); err != nil {
		t.Fatalf("clear custom_statuses: %v", err)
	}
	// Seed config rows directly (bypass SetConfig so we don't trigger the
	// runtime sync dispatch — we want to test that the migration alone
	// populates the mirror tables).
	if _, err := store.pool.Exec(ctx,
		`DELETE FROM config WHERE key IN ('types.custom','status.custom')`,
	); err != nil {
		t.Fatalf("clear config: %v", err)
	}
	if typesValue != "" {
		if _, err := store.pool.Exec(ctx,
			`INSERT INTO config (key, value) VALUES ('types.custom', $1)`, typesValue,
		); err != nil {
			t.Fatalf("seed types.custom: %v", err)
		}
	}
	if statusValue != "" {
		if _, err := store.pool.Exec(ctx,
			`INSERT INTO config (key, value) VALUES ('status.custom', $1)`, statusValue,
		); err != nil {
			t.Fatalf("seed status.custom: %v", err)
		}
	}
}

// schemaVersionRecorded reports whether bd_schema_migrations contains the row
// for v3.
func schemaVersionRecorded(t *testing.T, store *PostgresStore, version int) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM bd_schema_migrations WHERE version = $1", version,
	).Scan(&count); err != nil {
		t.Fatalf("read bd_schema_migrations: %v", err)
	}
	return count > 0
}

// TestMigration0003BackfillsEmptyTables exercises the empty-DB scenario:
// config rows present, mirror tables empty → migration populates both.
func TestMigration0003BackfillsEmptyTables(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	resetForMigrationV3Test(t, store, `["alpha","beta"]`, "reviewing:active,shipped:done")
	if err := store.runMigrations(ctx, false); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if !schemaVersionRecorded(t, store, 3) {
		t.Error("version 3 not recorded after migration")
	}

	types, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get types: %v", err)
	}
	if !equalStrings(types, []string{"alpha", "beta"}) {
		t.Errorf("custom_types after backfill: got %v, want [alpha beta]", types)
	}

	statuses, err := store.GetCustomStatusesDetailed(ctx)
	if err != nil {
		t.Fatalf("get statuses: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("custom_statuses after backfill: got %d rows, want 2", len(statuses))
	}
}

// TestMigration0003LeavesPopulatedTablesAlone exercises the count-short-circuit:
// if custom_types has rows, backfill must not touch them even if the config
// row says something else.
func TestMigration0003LeavesPopulatedTablesAlone(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	resetForMigrationV3Test(t, store, `["from_config"]`, "")
	// Pre-populate custom_types with a different name to prove backfill
	// short-circuits on count>0.
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO custom_types (name) VALUES ($1)", "from_manual_seed",
	); err != nil {
		t.Fatalf("seed custom_types: %v", err)
	}

	if err := store.runMigrations(ctx, false); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	types, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get types: %v", err)
	}
	if !equalStrings(types, []string{"from_manual_seed"}) {
		t.Errorf("custom_types: got %v, want [from_manual_seed]", types)
	}
}

// TestMigration0003TolerantOfCorruptStatusConfig exercises the parse-error
// skip path: a malformed status.custom value must be logged and skipped
// without aborting the migration. Version 3 still records.
func TestMigration0003TolerantOfCorruptStatusConfig(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	// Corrupt status.custom (invalid category); types.custom is fine.
	resetForMigrationV3Test(t, store, `["t1"]`, "reviewing:not-a-real-category")
	if err := store.runMigrations(ctx, false); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if !schemaVersionRecorded(t, store, 3) {
		t.Error("version 3 not recorded after migration with corrupt status.custom")
	}
	types, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get types: %v", err)
	}
	if !equalStrings(types, []string{"t1"}) {
		t.Errorf("custom_types: got %v, want [t1]", types)
	}
	statuses, err := store.GetCustomStatusesDetailed(ctx)
	if err != nil {
		t.Fatalf("get statuses: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("custom_statuses: got %d rows, want 0 (corrupt config should be skipped)", len(statuses))
	}
}

// TestMigration0003Idempotent exercises the re-run path: once v3 has applied,
// calling runMigrations again is a no-op (v3 row already present).
func TestMigration0003Idempotent(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	resetForMigrationV3Test(t, store, `["foo"]`, "")
	if err := store.runMigrations(ctx, false); err != nil {
		t.Fatalf("first runMigrations: %v", err)
	}
	if !schemaVersionRecorded(t, store, 3) {
		t.Fatal("v3 not recorded after first run")
	}

	// Mutate the mirror table to confirm the second run does NOT touch it.
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO custom_types (name) VALUES ($1)", "added_after_migration",
	); err != nil {
		t.Fatalf("post-migration seed: %v", err)
	}

	if err := store.runMigrations(ctx, false); err != nil {
		t.Fatalf("second runMigrations: %v", err)
	}
	types, err := store.GetCustomTypes(ctx)
	if err != nil {
		t.Fatalf("get types: %v", err)
	}
	if !equalStrings(types, []string{"added_after_migration", "foo"}) {
		t.Errorf("custom_types after re-run: got %v, want [added_after_migration foo]", types)
	}
}

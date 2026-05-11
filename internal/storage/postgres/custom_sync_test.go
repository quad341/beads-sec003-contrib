//go:build integration_pg

package postgres

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/steveyegge/beads/internal/storage"
)

// readCustomTypes returns the rows of custom_types as a sorted []string.
func readCustomTypes(ctx context.Context, tx pgx.Tx, t *testing.T) []string {
	t.Helper()
	rows, err := tx.Query(ctx, "SELECT name FROM custom_types ORDER BY name")
	if err != nil {
		t.Fatalf("read custom_types: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan custom_types: %v", err)
		}
		got = append(got, name)
	}
	return got
}

type statusRow struct {
	Name     string
	Category string
}

// readCustomStatuses returns the rows of custom_statuses sorted by name.
func readCustomStatuses(ctx context.Context, tx pgx.Tx, t *testing.T) []statusRow {
	t.Helper()
	rows, err := tx.Query(ctx, "SELECT name, category FROM custom_statuses ORDER BY name")
	if err != nil {
		t.Fatalf("read custom_statuses: %v", err)
	}
	defer rows.Close()
	var got []statusRow
	for rows.Next() {
		var s statusRow
		if err := rows.Scan(&s.Name, &s.Category); err != nil {
			t.Fatalf("scan custom_statuses: %v", err)
		}
		got = append(got, s)
	}
	return got
}

// withTx runs fn inside a fresh tx against the test store. The tx is rolled
// back on exit so each subtest sees a clean table.
func withTx(t *testing.T, store *PostgresStore, fn func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fn(ctx, tx)
}

// TestSyncCustomTypesPg covers the syncCustomTypesPg contract: empty value
// clears the table, JSON-array and comma-separated inputs both populate it,
// and a subsequent sync fully replaces prior contents.
func TestSyncCustomTypesPg(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	t.Run("empty value clears table", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, "INSERT INTO custom_types (name) VALUES ($1)", "preexisting"); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if err := syncCustomTypesPg(ctx, tx, ""); err != nil {
				t.Fatalf("sync empty: %v", err)
			}
			if got := readCustomTypes(ctx, tx, t); len(got) != 0 {
				t.Errorf("expected empty table, got %v", got)
			}
		})
	})

	t.Run("JSON array inserts exact rows", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			if err := syncCustomTypesPg(ctx, tx, `["gate","convoy"]`); err != nil {
				t.Fatalf("sync: %v", err)
			}
			want := []string{"convoy", "gate"}
			got := readCustomTypes(ctx, tx, t)
			if !equalStrings(got, want) {
				t.Errorf("rows: got %v, want %v", got, want)
			}
		})
	})

	t.Run("comma-separated inserts exact rows", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			if err := syncCustomTypesPg(ctx, tx, "alpha, beta ,gamma"); err != nil {
				t.Fatalf("sync: %v", err)
			}
			want := []string{"alpha", "beta", "gamma"}
			got := readCustomTypes(ctx, tx, t)
			if !equalStrings(got, want) {
				t.Errorf("rows: got %v, want %v", got, want)
			}
		})
	})

	t.Run("re-sync replaces prior contents", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			if err := syncCustomTypesPg(ctx, tx, "one,two"); err != nil {
				t.Fatalf("first sync: %v", err)
			}
			if err := syncCustomTypesPg(ctx, tx, "three"); err != nil {
				t.Fatalf("second sync: %v", err)
			}
			want := []string{"three"}
			got := readCustomTypes(ctx, tx, t)
			if !equalStrings(got, want) {
				t.Errorf("rows: got %v, want %v", got, want)
			}
		})
	})
}

// TestSyncCustomStatusesPg covers the syncCustomStatusesPg contract: empty
// value clears the table, category-aware "name:category" entries land with
// the right Category column value, and malformed inputs return a wrapped
// error whose chain mentions "parse status.custom".
func TestSyncCustomStatusesPg(t *testing.T) {
	dsn, stop := startPG(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := openStore(ctx, storage.ConnectionConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	t.Run("empty value clears table", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, "INSERT INTO custom_statuses (name, category) VALUES ($1, $2)",
				"preexisting", ""); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if err := syncCustomStatusesPg(ctx, tx, ""); err != nil {
				t.Fatalf("sync empty: %v", err)
			}
			if got := readCustomStatuses(ctx, tx, t); len(got) != 0 {
				t.Errorf("expected empty table, got %v", got)
			}
		})
	})

	t.Run("category-aware format inserts correctly", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			if err := syncCustomStatusesPg(ctx, tx, "reviewing:active,shipped:done"); err != nil {
				t.Fatalf("sync: %v", err)
			}
			got := readCustomStatuses(ctx, tx, t)
			want := []statusRow{
				{Name: "reviewing", Category: "active"},
				{Name: "shipped", Category: "done"},
			}
			if !equalStatusRows(got, want) {
				t.Errorf("rows: got %v, want %v", got, want)
			}
		})
	})

	t.Run("malformed value returns wrapped error", func(t *testing.T) {
		withTx(t, store, func(ctx context.Context, tx pgx.Tx) {
			err := syncCustomStatusesPg(ctx, tx, "reviewing:not-a-real-category")
			if err == nil {
				t.Fatal("expected error for malformed status category, got nil")
			}
			if !strings.Contains(err.Error(), "parse status.custom") {
				t.Errorf("expected error chain to mention 'parse status.custom': %q", err.Error())
			}
		})
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ca := append([]string(nil), a...)
	cb := append([]string(nil), b...)
	sort.Strings(ca)
	sort.Strings(cb)
	for i := range ca {
		if ca[i] != cb[i] {
			return false
		}
	}
	return true
}

func equalStatusRows(a, b []statusRow) bool {
	if len(a) != len(b) {
		return false
	}
	ca := append([]statusRow(nil), a...)
	cb := append([]statusRow(nil), b...)
	sort.Slice(ca, func(i, j int) bool { return ca[i].Name < ca[j].Name })
	sort.Slice(cb, func(i, j int) bool { return cb[i].Name < cb[j].Name })
	for i := range ca {
		if ca[i] != cb[i] {
			return false
		}
	}
	return true
}

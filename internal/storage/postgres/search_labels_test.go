//go:build integration_pg

package postgres

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/postgres/testfixture"
	"github.com/steveyegge/beads/internal/types"
)

// queryCounter counts SQL queries executed through a pgx connection.
// Satisfies pgx.QueryTracer.
type queryCounter struct {
	n atomic.Int64
}

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.n.Add(1)
	return ctx
}

func (q *queryCounter) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {}

func (q *queryCounter) reset()       { q.n.Store(0) }
func (q *queryCounter) count() int64 { return q.n.Load() }

// openStoreWithCounter opens a PostgresStore with a query counter wired into
// the pgx tracer. Callers get both the store and the counter so they can
// reset and read the count around specific operations.
func openStoreWithCounter(ctx context.Context, dsn string) (*PostgresStore, *queryCounter, error) {
	pgxCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse dsn: %w", err)
	}

	counter := &queryCounter{}
	pgxCfg.ConnConfig.Tracer = counter

	// Apply the same defaults openStore uses.
	if pgxCfg.MaxConns == 0 {
		pgxCfg.MaxConns = defaultMaxConns
	}
	if pgxCfg.MaxConnLifetime == 0 {
		pgxCfg.MaxConnLifetime = defaultMaxConnLifetime
	}
	if pgxCfg.MaxConnIdleTime == 0 {
		pgxCfg.MaxConnIdleTime = defaultMaxConnIdleTime
	}
	pgxCfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET TIME ZONE 'UTC'")
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("new pool: %w", err)
	}

	store := &PostgresStore{pool: pool}
	if err := store.checkVersion(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}
	if err := store.runMigrations(ctx, false); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return store, counter, nil
}

// TestSearchIssuesBulkLabels verifies that SearchIssues issues at most 2 SQL
// queries regardless of how many issues are returned — 1 for the issues table
// scan and 1 for the bulk label fetch. Before the fix this was N+1.
func TestSearchIssuesBulkLabels(t *testing.T) {
	dsn := testfixture.ForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, counter, err := openStoreWithCounter(ctx, dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.SetConfig(ctx, "issue_prefix", "bd"); err != nil {
		t.Fatalf("set prefix: %v", err)
	}

	// Create 50 issues each with 2 labels.
	const numIssues = 50
	for i := 0; i < numIssues; i++ {
		issue := &types.Issue{
			Title:     fmt.Sprintf("issue-%d", i),
			IssueType: types.TypeTask,
			Priority:  2,
			Status:    types.StatusOpen,
		}
		if err := store.CreateIssue(ctx, issue, "tester"); err != nil {
			t.Fatalf("create issue %d: %v", i, err)
		}
		for _, label := range []string{"alpha", "beta"} {
			if err := store.AddLabel(ctx, issue.ID, label, "tester"); err != nil {
				t.Fatalf("add label to %s: %v", issue.ID, err)
			}
		}
	}

	// Reset counter so setup queries don't skew the assertion.
	counter.reset()

	issues, err := store.SearchIssues(ctx, "", types.IssueFilter{})
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(issues) != numIssues {
		t.Errorf("want %d issues, got %d", numIssues, len(issues))
	}

	// Verify each issue has its labels populated.
	for _, iss := range issues {
		if len(iss.Labels) != 2 {
			t.Errorf("issue %s: want 2 labels, got %d", iss.ID, len(iss.Labels))
		}
	}

	got := counter.count()
	if got > 2 {
		t.Errorf("SearchIssues fired %d SQL queries for %d issues; want ≤ 2 (1 scan + 1 bulk labels)", got, numIssues)
	}
}

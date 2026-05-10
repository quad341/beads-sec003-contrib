// Package postgres — iter_issues.go
//
// Streaming iterator over the issues table. Holds a DEDICATED pool
// connection for the cursor's lifetime so per-row label hydration can
// run on a SECOND pool connection without deadlocking against the
// cursor connection (be-jaavsb §4.1).
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// pgIssueIter is a streaming iterator over the issues table.
//
// It holds a dedicated *pgxpool.Conn for its lifetime: the cursor
// connection is busy holding pgx.Rows so per-row label hydration must
// run on a SECOND connection from the pool. Forgetting this is the
// classic deadlock — the iterator is locked, the hydration query waits
// for the same conn, and the pool blocks forever. The dedicated-conn
// pattern is pinned by the parity test in iter_test.go (concurrent
// iterators).
type pgIssueIter struct {
	s      *PostgresStore
	cursor *pgxpool.Conn
	rows   pgx.Rows
	cur    *types.Issue
	err    error
	closed bool
}

// IterIssues streams issues matching the filter. The filter.Limit is
// honored if set; the iterator path is also valid for bounded queries
// when the caller wants streaming.
func (s *PostgresStore) IterIssues(ctx context.Context, query string, filter types.IssueFilter) (storage.Iter[types.Issue], error) {
	where, args, err := buildSearchClause(query, filter)
	if err != nil {
		return nil, err
	}
	limit := ""
	if filter.Limit > 0 {
		limit = fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	//nolint:gosec // clauses use only parameter placeholders, identifiers are static
	q := fmt.Sprintf(`SELECT %s FROM issues%s ORDER BY created_at DESC%s`, issueColumns, where, limit)

	cursor, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, wrapErr("acquire pg conn for iter issues", err)
	}
	rows, err := cursor.Query(ctx, q, args...)
	if err != nil {
		cursor.Release()
		return nil, wrapErr("iter issues query", err)
	}
	return &pgIssueIter{s: s, cursor: cursor, rows: rows}, nil
}

func (it *pgIssueIter) Next(ctx context.Context) bool {
	if it.err != nil || it.closed {
		return false
	}
	if err := ctx.Err(); err != nil {
		it.err = err
		return false
	}
	if !it.rows.Next() {
		it.err = it.rows.Err()
		return false
	}
	var r issueScanRow
	if err := it.rows.Scan(r.dest()...); err != nil {
		it.err = wrapErr("scan issue in iter", err)
		return false
	}
	iss := r.toIssue()
	// Hydrate labels on a SEPARATE connection (the pool — NOT the cursor conn).
	// The cursor conn is busy holding the rows. Using s.pool acquires a fresh
	// conn for the duration of getLabelsFromTable.
	labels, err := getLabelsFromTable(ctx, it.s.pool, "labels", iss.ID)
	if err != nil {
		it.err = wrapErr("hydrate labels in iter issues", err)
		return false
	}
	iss.Labels = labels
	it.cur = iss
	return true
}

func (it *pgIssueIter) Value() *types.Issue { return it.cur }
func (it *pgIssueIter) Err() error          { return it.err }

func (it *pgIssueIter) Close() error {
	if it.closed {
		return nil
	}
	it.closed = true
	if it.rows != nil {
		it.rows.Close()
	}
	if it.cursor != nil {
		it.cursor.Release()
	}
	return nil
}

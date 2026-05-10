// Package postgres — iter_dependents.go
//
// Streaming iterator over the dependents/dependencies-with-metadata join.
// Same dedicated-conn pattern as iter_issues.go — labels hydrate on a
// second pool connection so the cursor never deadlocks.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// pgDependentsIter streams the issues × dependency join, returning each
// row as an IssueWithDependencyMetadata. Used by both IterDependents-
// WithMetadata and IterDependenciesWithMetadata; the SQL differs only in
// which side of the dependencies join is bound to issueID.
type pgDependentsIter struct {
	s      *PostgresStore
	cursor *pgxpool.Conn
	rows   pgx.Rows
	cur    *types.IssueWithDependencyMetadata
	err    error
	closed bool
}

// IterDependentsWithMetadata streams dependents (issues that depend on
// issueID) with the relationship type attached. Mirror of
// GetDependentsWithMetadata.
func (s *PostgresStore) IterDependentsWithMetadata(ctx context.Context, issueID string) (storage.Iter[types.IssueWithDependencyMetadata], error) {
	q := fmt.Sprintf(`
		SELECT %s, d.type
		FROM issues i
		JOIN dependencies d ON d.issue_id = i.id
		WHERE d.depends_on_id = $1
		ORDER BY i.created_at ASC
	`, prefixedIssueColumns("i"))
	return s.iterIssuesWithDepType(ctx, q, issueID)
}

// IterDependenciesWithMetadata streams dependencies (issues issueID
// depends on) with the relationship type attached. Mirror of
// GetDependenciesWithMetadata.
func (s *PostgresStore) IterDependenciesWithMetadata(ctx context.Context, issueID string) (storage.Iter[types.IssueWithDependencyMetadata], error) {
	q := fmt.Sprintf(`
		SELECT %s, d.type
		FROM issues i
		JOIN dependencies d ON d.depends_on_id = i.id
		WHERE d.issue_id = $1
		ORDER BY i.created_at ASC
	`, prefixedIssueColumns("i"))
	return s.iterIssuesWithDepType(ctx, q, issueID)
}

func (s *PostgresStore) iterIssuesWithDepType(ctx context.Context, q string, args ...any) (storage.Iter[types.IssueWithDependencyMetadata], error) {
	cursor, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, wrapErr("acquire pg conn for iter dependents", err)
	}
	rows, err := cursor.Query(ctx, q, args...)
	if err != nil {
		cursor.Release()
		return nil, wrapErr("iter dependents query", err)
	}
	return &pgDependentsIter{s: s, cursor: cursor, rows: rows}, nil
}

func (it *pgDependentsIter) Next(ctx context.Context) bool {
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
	var depType string
	dest := append(r.dest(), &depType)
	if err := it.rows.Scan(dest...); err != nil {
		it.err = wrapErr("scan issues with dep type in iter", err)
		return false
	}
	// NOTE: labels are NOT hydrated here. The corresponding slice path
	// (scanIssuesWithDepType in dependencies.go) does not hydrate either,
	// so this matches for parity. Callers that need labels on dependents
	// must call GetLabels(id) explicitly.
	it.cur = &types.IssueWithDependencyMetadata{
		Issue:          *r.toIssue(),
		DependencyType: types.DependencyType(depType),
	}
	return true
}

func (it *pgDependentsIter) Value() *types.IssueWithDependencyMetadata { return it.cur }
func (it *pgDependentsIter) Err() error                                { return it.err }

func (it *pgDependentsIter) Close() error {
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

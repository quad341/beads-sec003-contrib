package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// maxDeleteBySourceRepoIDs caps the per-call set of issues this verb will
// remove in one transaction. Operators with more than this many issues from a
// single source repo should drive cleanup through the sync path instead, where
// the work can be paged. See bead be-ptzlki §5 for the design rationale.
const maxDeleteBySourceRepoIDs = 100_000

// DeleteIssuesBySourceRepo removes every issue whose source_repo column equals
// sourceRepo, plus every inbound dependency edge that referenced one of those
// issues. Used by `bd repo remove` (cmd/bd/repo.go:123) to evict the issues
// for a removed multi-repo source.
//
// Behavior tightening vs Dolt: an empty sourceRepo returns an error rather
// than silently deleting every row with source_repo=”. The Dolt path in
// internal/storage/issueops/bulk_ops.go:175 does the silent wipe; flagging
// for reviewer signoff is called out in the be-efr0tm builder commit.
//
// Schema win: every issues-side child table declares ON DELETE CASCADE in
// migrations/0001_initial.up.sql, so `DELETE FROM issues WHERE source_repo=$1`
// cascades labels/comments/events/dependencies(out)/issue_snapshots/
// compaction_snapshots automatically. Only dependencies.depends_on_id (no FK)
// needs an explicit pre-pass.
//
// Wisps are out of scope: the Dolt reference does not scrub wisps.source_repo
// (issueops/bulk_ops.go:175 only touches the `issues` table), so neither does
// this. See bead be-ptzlki §5.
func (s *PostgresStore) DeleteIssuesBySourceRepo(ctx context.Context, sourceRepo string) (int, error) {
	if sourceRepo == "" {
		return 0, errors.New("postgres: DeleteIssuesBySourceRepo requires non-empty sourceRepo")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, wrapErr("begin delete-by-source-repo transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := tx.Query(ctx, "SELECT id FROM issues WHERE source_repo = $1", sourceRepo)
	if err != nil {
		return 0, wrapErr("select issue ids by source_repo", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return 0, wrapErr("scan issue id for source_repo", scanErr)
		}
		ids = append(ids, id)
		if len(ids) > maxDeleteBySourceRepoIDs {
			rows.Close()
			return 0, fmt.Errorf("postgres: source repo %q has >%d issues; use sync to drain instead of DeleteIssuesBySourceRepo",
				sourceRepo, maxDeleteBySourceRepoIDs)
		}
	}
	rows.Close()
	if rerr := rows.Err(); rerr != nil {
		return 0, wrapErr("iterate issue ids for source_repo", rerr)
	}

	if len(ids) == 0 {
		return 0, nil
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM dependencies WHERE depends_on_id = ANY($1)", ids); err != nil {
		return 0, wrapErr("delete inbound dependencies by source_repo", err)
	}

	cmd, err := tx.Exec(ctx, "DELETE FROM issues WHERE source_repo = $1", sourceRepo)
	if err != nil {
		return 0, wrapErr("delete issues by source_repo", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, wrapErr("commit delete-by-source-repo transaction", err)
	}
	return int(cmd.RowsAffected()), nil
}

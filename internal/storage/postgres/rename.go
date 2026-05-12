package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/steveyegge/beads/internal/types"
)

// pgUniqueViolation is SQLSTATE 23505 — raised on PK / UNIQUE conflict. We map
// this to a typed "target ID already in use" error so cmd/bd/rename.go can wrap
// it for a user-facing message.
const pgUniqueViolation = "23505"

// UpdateIssueID renames an issue and migrates every cross-table reference.
// Mirrors the Dolt-side semantics in internal/storage/issueops/bulk_ops.go:220
// (UpdateIssueIDInTx). See bead be-ptzlki for the design and bead be-lj68rq
// for the PG-specific build notes.
//
// The new content (title/description/design/acceptance_criteria/notes) on
// `issue` is applied in the same statement that changes the ID, matching the
// Dolt path and the cmd/bd/rename.go:80 call site contract.
//
// Implementation strategy: migration 0004 added ON UPDATE CASCADE to the six
// issues-side FKs, so updating issues.id automatically migrates the rows in
// labels / comments / events / dependencies(issue_id) / issue_snapshots /
// compaction_snapshots. Only two columns need manual UPDATEs because they have
// no FK back to issues.id:
//   - dependencies.depends_on_id (no FK, intentional — inbound edges)
//   - child_counters.parent_id   (FK dropped in migration 0002)
//
// Wisp tables have no FKs at all, so the wisp branch uses manual UPDATEs for
// each of the five wisp_* tables.
func (s *PostgresStore) UpdateIssueID(ctx context.Context, oldID, newID string, issue *types.Issue, actor string) error {
	if oldID == newID {
		return nil
	}
	if issue == nil {
		return errors.New("postgres: UpdateIssueID: issue must not be nil")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return wrapErr("begin rename transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if isActiveWispID(ctx, tx, oldID) {
		if err := updateWispIDInTxPG(ctx, tx, oldID, newID, issue, actor); err != nil {
			return err
		}
	} else {
		if err := updateIssueIDInTxPG(ctx, tx, oldID, newID, issue, actor); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapErr("commit rename transaction", err)
	}
	return nil
}

// updateIssueIDInTxPG handles the regular (persistent) rename branch.
func updateIssueIDInTxPG(ctx context.Context, tx pgx.Tx, oldID, newID string, issue *types.Issue, actor string) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE issues
		SET id = $1,
		    title = $2,
		    description = $3,
		    design = $4,
		    acceptance_criteria = $5,
		    notes = $6,
		    updated_at = NOW()
		WHERE id = $7
	`,
		newID,
		issue.Title,
		issue.Description,
		issue.Design,
		issue.AcceptanceCriteria,
		issue.Notes,
		oldID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("target ID already in use: %s", newID)
		}
		return wrapErr("update issue id", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("issue not found: %s", oldID)
	}

	// FK CASCADE handled: dependencies.issue_id, labels.issue_id,
	// comments.issue_id, events.issue_id, issue_snapshots.issue_id,
	// compaction_snapshots.issue_id.
	//
	// Manual updates for columns with no FK back to issues.id:
	manualRefs := []struct {
		table string
		col   string
	}{
		{"dependencies", "depends_on_id"},
		{"child_counters", "parent_id"},
		{"wisp_dependencies", "issue_id"},
		{"wisp_dependencies", "depends_on_id"},
		{"wisp_events", "issue_id"},
		{"wisp_labels", "issue_id"},
		{"wisp_comments", "issue_id"},
	}
	for _, r := range manualRefs {
		//nolint:gosec // table/column names are hardcoded
		stmt := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2", r.table, r.col, r.col)
		if _, err := tx.Exec(ctx, stmt, newID, oldID); err != nil {
			return wrapErr(fmt.Sprintf("update %s.%s", r.table, r.col), err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (issue_id, event_type, actor, old_value, new_value)
		VALUES ($1, 'renamed', $2, $3, $4)
	`, newID, actor, oldID, newID); err != nil {
		return wrapErr("record rename event", err)
	}
	return nil
}

// updateWispIDInTxPG handles the wisp (ephemeral) rename branch. Wisp tables
// have no FK constraints, so every reference is updated manually.
func updateWispIDInTxPG(ctx context.Context, tx pgx.Tx, oldID, newID string, issue *types.Issue, actor string) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE wisps
		SET id = $1,
		    title = $2,
		    description = $3,
		    design = $4,
		    acceptance_criteria = $5,
		    notes = $6,
		    updated_at = NOW()
		WHERE id = $7
	`,
		newID,
		issue.Title,
		issue.Description,
		issue.Design,
		issue.AcceptanceCriteria,
		issue.Notes,
		oldID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("target ID already in use: %s", newID)
		}
		return wrapErr("update wisp id", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("wisp not found: %s", oldID)
	}

	manualRefs := []struct {
		table string
		col   string
	}{
		{"wisp_dependencies", "issue_id"},
		{"wisp_dependencies", "depends_on_id"},
		{"wisp_events", "issue_id"},
		{"wisp_labels", "issue_id"},
		{"wisp_comments", "issue_id"},
	}
	for _, r := range manualRefs {
		//nolint:gosec // table/column names are hardcoded
		stmt := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2", r.table, r.col, r.col)
		if _, err := tx.Exec(ctx, stmt, newID, oldID); err != nil {
			return wrapErr(fmt.Sprintf("update %s.%s", r.table, r.col), err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO wisp_events (issue_id, event_type, actor, old_value, new_value)
		VALUES ($1, 'renamed', $2, $3, $4)
	`, newID, actor, oldID, newID); err != nil {
		return wrapErr("record wisp rename event", err)
	}
	return nil
}

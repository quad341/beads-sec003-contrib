package issueops

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// HistoryInTx returns the complete version history for an issue by querying
// the dolt_diff_issues system table. The result is ordered newest-first.
//
// dolt_diff_issues emits one row per (PK, commit) only when that commit
// actually changed the PK's data, unlike dolt_history_issues (which walks
// every commit reachable from HEAD and emits a row for any PK that existed
// in that commit's snapshot, regardless of whether the commit touched it —
// so an edit to a different issue bled a ghost row into this issue's
// history). to_X columns hold the post-change value; for a 'removed' diff
// (the issue was deleted) to_X is NULL, so COALESCE falls back to from_X to
// still surface a final entry for the deletion commit itself. to_commit is
// always the commit responsible for the row's change, across all three
// diff types, so it (not from_commit) is the entry's commit_hash.
//
// Merge commits (2+ rows in dolt_commit_ancestors for the same commit_hash)
// are excluded. Dolt computes a merge commit's own diff against its first
// parent only, so a change that arrived solely via the merged-in (second)
// parent is re-stated as a second, duplicate diff row on the merge commit
// itself -- alongside the correctly-attributed row on the original commit
// that actually made the change. Excluding merge commits keeps the one row
// that carries the real authorship/timestamp and drops the redundant echo.
//
// The subquery wrapper avoids Dolt's max1Row optimization on PK lookup:
// dolt_diff_* tables can return multiple rows per PK (one per commit that
// changed it), but the query planner can incorrectly assume WHERE id=?
// returns one row.
func HistoryInTx(ctx context.Context, tx DBTX, issueID string) ([]*storage.HistoryEntry, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT
			id, title, description, design, acceptance_criteria, notes,
			status, priority, issue_type, assignee, owner, created_by,
			estimated_minutes, created_at, updated_at, closed_at, close_reason,
			pinned, mol_type, diff_type,
			commit_hash, committer, commit_date
		FROM (
			SELECT
				COALESCE(d.to_id, d.from_id) AS id,
				COALESCE(d.to_title, d.from_title) AS title,
				COALESCE(d.to_description, d.from_description, '') AS description,
				COALESCE(d.to_design, d.from_design, '') AS design,
				COALESCE(d.to_acceptance_criteria, d.from_acceptance_criteria, '') AS acceptance_criteria,
				COALESCE(d.to_notes, d.from_notes, '') AS notes,
				COALESCE(d.to_status, d.from_status) AS status,
				COALESCE(d.to_priority, d.from_priority) AS priority,
				COALESCE(d.to_issue_type, d.from_issue_type) AS issue_type,
				COALESCE(d.to_assignee, d.from_assignee) AS assignee,
				COALESCE(d.to_owner, d.from_owner) AS owner,
				COALESCE(d.to_created_by, d.from_created_by) AS created_by,
				COALESCE(d.to_estimated_minutes, d.from_estimated_minutes) AS estimated_minutes,
				COALESCE(d.to_created_at, d.from_created_at) AS created_at,
				COALESCE(d.to_updated_at, d.from_updated_at) AS updated_at,
				COALESCE(d.to_closed_at, d.from_closed_at) AS closed_at,
				COALESCE(d.to_close_reason, d.from_close_reason) AS close_reason,
				COALESCE(d.to_pinned, d.from_pinned) AS pinned,
				COALESCE(d.to_mol_type, d.from_mol_type) AS mol_type,
				d.diff_type,
				d.to_commit AS commit_hash,
				l.committer AS committer,
				d.to_commit_date AS commit_date
			FROM dolt_diff_issues d
			JOIN dolt_log l ON l.commit_hash = d.to_commit
			WHERE d.to_commit NOT IN (
				SELECT commit_hash FROM dolt_commit_ancestors
				GROUP BY commit_hash
				HAVING COUNT(*) > 1
			)
		) h
		WHERE h.id = ?
		ORDER BY h.commit_date DESC
	`, issueID)
	if err != nil {
		return nil, fmt.Errorf("failed to get issue history: %w", err)
	}
	defer rows.Close()

	var entries []*storage.HistoryEntry
	for rows.Next() {
		var issue types.Issue
		var createdAtStr, updatedAtStr sql.NullString
		var closedAt sql.NullTime
		var assignee, owner, createdBy, closeReason, molType sql.NullString
		var estimatedMinutes sql.NullInt64
		var pinned sql.NullInt64
		var diffType, commitHash, committer string
		var commitDate time.Time

		if err := rows.Scan(
			&issue.ID, &issue.Title, &issue.Description, &issue.Design, &issue.AcceptanceCriteria, &issue.Notes,
			&issue.Status, &issue.Priority, &issue.IssueType, &assignee, &owner, &createdBy,
			&estimatedMinutes, &createdAtStr, &updatedAtStr, &closedAt, &closeReason,
			&pinned, &molType, &diffType,
			&commitHash, &committer, &commitDate,
		); err != nil {
			return nil, fmt.Errorf("failed to scan history: %w", err)
		}

		if createdAtStr.Valid {
			issue.CreatedAt = ParseTimeString(createdAtStr.String)
		}
		if updatedAtStr.Valid {
			issue.UpdatedAt = ParseTimeString(updatedAtStr.String)
		}
		if closedAt.Valid {
			issue.ClosedAt = &closedAt.Time
		}
		if assignee.Valid {
			issue.Assignee = assignee.String
		}
		if owner.Valid {
			issue.Owner = owner.String
		}
		if createdBy.Valid {
			issue.CreatedBy = createdBy.String
		}
		if estimatedMinutes.Valid {
			mins := int(estimatedMinutes.Int64)
			issue.EstimatedMinutes = &mins
		}
		if closeReason.Valid {
			issue.CloseReason = closeReason.String
		}
		if pinned.Valid && pinned.Int64 != 0 {
			issue.Pinned = true
		}
		if molType.Valid {
			issue.MolType = types.MolType(molType.String)
		}

		entries = append(entries, &storage.HistoryEntry{
			CommitHash: commitHash,
			Committer:  committer,
			CommitDate: commitDate,
			Issue:      &issue,
			DiffType:   diffType,
		})
	}

	return entries, rows.Err()
}

// PreviousExternalRefInTx returns the external_ref value recorded for
// issueID as of the most recent commit at or before asOf, by querying the
// dolt_diff_issues system table. found is false if no history entry exists
// for issueID at or before asOf.
//
// See HistoryInTx above for why dolt_diff_issues (not dolt_history_issues)
// is correct here, why to_X falls back to from_X via COALESCE, and why merge
// commits are excluded (same duplicate-row mechanism; here a duplicate can
// also shift which row wins the "most recent at or before asOf" tiebreak).
//
// The subquery wrapper avoids Dolt's max1Row optimization on PK lookup, for
// the same reason described on HistoryInTx above.
func PreviousExternalRefInTx(ctx context.Context, tx *sql.Tx, issueID string, asOf time.Time) (string, bool, error) {
	var previousRef sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT external_ref
		FROM (
			SELECT
				COALESCE(d.to_id, d.from_id) AS id,
				COALESCE(d.to_external_ref, d.from_external_ref) AS external_ref,
				d.to_commit_date AS commit_date
			FROM dolt_diff_issues d
			WHERE d.to_commit NOT IN (
				SELECT commit_hash FROM dolt_commit_ancestors
				GROUP BY commit_hash
				HAVING COUNT(*) > 1
			)
		) h
		WHERE h.id = ? AND h.commit_date <= ?
		ORDER BY h.commit_date DESC
		LIMIT 1
	`, issueID, asOf.UTC()).Scan(&previousRef)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to get previous external_ref: %w", err)
	}
	return previousRef.String, true, nil
}

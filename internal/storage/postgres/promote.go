package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// PromoteFromEphemeral converts a wisp into a permanent issue. The row is
// copied from `wisps` to `issues`, the four aux tables are migrated
// (`wisp_labels`→`labels`, `wisp_dependencies`→`dependencies` (with a target-
// existence filter that drops dangling edges — see note below),
// `wisp_events`→`events`, `wisp_comments`→`comments`), then every wisp_* row
// referencing the id is deleted. The whole sequence runs in one PG
// transaction at READ COMMITTED.
//
// Behavior tightening vs the Dolt backend: when a wisp_dependency points at
// a target that is not yet present in `issues`, this implementation silently
// drops the edge via an EXISTS filter. The Dolt path emits the INSERT and
// swallows the resulting FK-violation error string. Both paths surface "lost
// dep edges" the same way from the caller's perspective, but the PG path is
// explicit. See bead be-gzztuj for context.
//
// Event/comment copies are best-effort: each runs under its own savepoint so
// a malformed wisp_event row (e.g., a value that fails column constraints
// the wisp tables did not enforce) does not abort the promote. The outer tx
// still commits; the failure is logged.
func (s *PostgresStore) PromoteFromEphemeral(ctx context.Context, id, actor string) error {
	if s.IsClosed() {
		return errors.New("postgres: store is closed")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return wrapErr("promote: begin", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	// Step 1: read the wisp row.
	issue, err := getIssueFrom(ctx, tx, "wisps", id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("wisp %s not found", id)
		}
		return err
	}

	// Step 2: refuse if the same ID already exists as a permanent issue. Both
	// reads share one tx so we never observe an issues row that was deleted
	// after we read the wisps row.
	if _, err := getIssueFrom(ctx, tx, "issues", id); err == nil {
		return fmt.Errorf("issue %s already exists in permanent table", id)
	} else if !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	// Step 3: clear ephemeral routing flags so the inserted row lands as a
	// regular issue (and so a fresh GetIssue won't fall back to wisps).
	issue.Ephemeral = false
	issue.NoHistory = false

	// Step 4: insert into the permanent issues table. insertIssueRow has an
	// ON CONFLICT DO UPDATE clause; in the rare concurrent-promote case the
	// second writer overwrites with identical wisp data, which is idempotent.
	// The non-race "issue already exists from a different source" case is
	// blocked by the step-2 pre-check above.
	if err := insertIssueRow(ctx, tx, "issues", issue); err != nil {
		return err
	}

	// Step 4b: record a fresh `created` event so the permanent issue's audit
	// trail starts at the promote moment (Dolt's CreateIssue path emits this
	// implicitly; we bypass CreateIssue to preserve the wisp's created_at, so
	// we emit it explicitly here).
	if err := recordEvent(ctx, tx, "events", id, types.EventCreated, actor, "", ""); err != nil {
		return err
	}

	// Step 5: copy labels. PK is (issue_id, label); ON CONFLICT defends
	// against legacy duplicates that snuck into wisp_labels.
	if _, err := tx.Exec(ctx, `
		INSERT INTO labels (issue_id, label)
		SELECT issue_id, label FROM wisp_labels WHERE issue_id = $1
		ON CONFLICT (issue_id, label) DO NOTHING
	`, id); err != nil {
		return wrapErr("promote: copy labels", err)
	}

	// Step 6: copy dependencies, dropping edges whose target wasn't promoted
	// alongside this wisp (see comment at the top of the file).
	if _, err := tx.Exec(ctx, `
		INSERT INTO dependencies (
			issue_id, depends_on_id, type, created_at, created_by, metadata, thread_id
		)
		SELECT wd.issue_id, wd.depends_on_id, wd.type, wd.created_at, wd.created_by, wd.metadata, wd.thread_id
		FROM wisp_dependencies wd
		WHERE wd.issue_id = $1
		  AND EXISTS (SELECT 1 FROM issues WHERE id = wd.depends_on_id)
		ON CONFLICT (issue_id, depends_on_id) DO NOTHING
	`, id); err != nil {
		return wrapErr("promote: copy dependencies", err)
	}

	// Step 7: copy events under a savepoint.
	if err := copyAuxUnderSavepoint(ctx, tx, id, "promote_events", `
		INSERT INTO events (issue_id, event_type, actor, old_value, new_value, comment, created_at)
		SELECT issue_id, event_type, actor, old_value, new_value, comment, created_at
		FROM wisp_events WHERE issue_id = $1
	`, "events"); err != nil {
		return err
	}

	// Step 8: copy comments under a savepoint.
	if err := copyAuxUnderSavepoint(ctx, tx, id, "promote_comments", `
		INSERT INTO comments (issue_id, author, text, created_at)
		SELECT issue_id, author, text, created_at
		FROM wisp_comments WHERE issue_id = $1
	`, "comments"); err != nil {
		return err
	}

	// Step 9: clean up wisp aux rows then the wisp itself.
	for _, q := range []string{
		`DELETE FROM wisp_dependencies WHERE issue_id = $1 OR depends_on_id = $1`,
		`DELETE FROM wisp_events       WHERE issue_id = $1`,
		`DELETE FROM wisp_comments     WHERE issue_id = $1`,
		`DELETE FROM wisp_labels       WHERE issue_id = $1`,
	} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return wrapErr("promote: delete wisp aux", err)
		}
	}
	cmd, err := tx.Exec(ctx, `DELETE FROM wisps WHERE id = $1`, id)
	if err != nil {
		return wrapErr("promote: delete wisp", err)
	}
	if cmd.RowsAffected() != 1 {
		return fmt.Errorf("postgres: promote: expected 1 wisp deletion, got %d", cmd.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapErr("promote: commit", err)
	}
	committed = true
	return nil
}

// copyAuxUnderSavepoint runs an INSERT...SELECT for one aux table under a
// PG savepoint. If the INSERT fails, the savepoint is rolled back and the
// failure is logged — the outer transaction stays alive so the rest of the
// promote can complete. Mirrors the Dolt backend's "best-effort" event/
// comment copy semantics.
func copyAuxUnderSavepoint(ctx context.Context, tx pgx.Tx, id, sp, insertSQL, label string) error {
	if _, err := tx.Exec(ctx, "SAVEPOINT "+sp); err != nil {
		return wrapErr("promote: savepoint "+label, err)
	}
	if _, err := tx.Exec(ctx, insertSQL, id); err != nil {
		log.Printf("promote %s: failed to copy %s (data may be lost): %v", id, label, err)
		if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+sp); rbErr != nil {
			return wrapErr("promote: rollback "+label+" savepoint", rbErr)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
		return wrapErr("promote: release "+label+" savepoint", err)
	}
	return nil
}

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steveyegge/beads/internal/types"
)

// deleteBatchSizePG bounds the IN-clause length for delete-path queries.
// Mirrors issueops.deleteBatchSize so PG paging behavior matches Dolt.
const deleteBatchSizePG = 50

// maxRecursiveResultsPG caps the total set discovered by a cascade walk.
// Mirrors issueops.maxRecursiveResults — guards against pathological cycles
// and avoids unbounded memory growth on large graphs.
const maxRecursiveResultsPG = 10000

// wispDeleteBatchPG is the per-transaction batch size for wisp deletion.
// Matches the design's ≤500 cap and the Dolt-side perf reasoning in
// dolt/wisps.go (separate transactions per batch to bound write timeout).
const wispDeleteBatchPG = 500

// DeleteIssues removes the given issues, mirroring the Dolt-side semantics
// in internal/storage/dolt/issues.go:386 and the shared in-tx helper in
// internal/storage/issueops/delete.go:71. See bead be-ptzlki for the design.
//
// Behavior:
//   - cascade=true: recursively delete dependents (capped at maxRecursiveResultsPG).
//   - cascade=false, force=false: error if any ID has dependents outside the
//     set; result.OrphanedIssues lists them. No writes.
//   - cascade=false, force=true: delete the explicit set; result.OrphanedIssues
//     lists dependents that lose an inbound edge.
//   - dryRun=true: populate stats and OrphanedIssues; no writes.
//
// PG schema declares ON DELETE CASCADE for the child tables of issues.id, so
// only dependencies.depends_on_id (no inverse FK) and the wisp_* tables (no
// FKs) need explicit cleanup. There is no PG analog of the Dolt
// invalidateBlockedIDsCache call.
func (s *PostgresStore) DeleteIssues(ctx context.Context, ids []string, cascade, force, dryRun bool) (*types.DeleteIssuesResult, error) {
	if len(ids) == 0 {
		return &types.DeleteIssuesResult{}, nil
	}

	wispIDs, regularIDs, err := partitionWispIDsPG(ctx, s.pool, ids)
	if err != nil {
		return nil, fmt.Errorf("partition wisps: %w", err)
	}

	wispDeleteCount := 0
	if len(wispIDs) > 0 {
		if dryRun {
			wispDeleteCount = len(wispIDs)
		} else {
			n, err := deleteWispsBatchedPG(ctx, s.pool, wispIDs)
			if err != nil {
				return nil, fmt.Errorf("delete wisps: %w", err)
			}
			wispDeleteCount = n
		}
	}

	if len(regularIDs) == 0 {
		return &types.DeleteIssuesResult{DeletedCount: wispDeleteCount}, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, wrapErr("begin delete transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	result, err := deleteRegularIssuesInTxPG(ctx, tx, regularIDs, cascade, force, dryRun)
	if err != nil {
		if result != nil {
			result.DeletedCount += wispDeleteCount
		}
		return result, err
	}

	if dryRun {
		result.DeletedCount += wispDeleteCount
		return result, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, wrapErr("commit delete transaction", err)
	}

	result.DeletedCount += wispDeleteCount
	return result, nil
}

// partitionWispIDsPG splits ids into wisp IDs (rows in `wisps`) and regular
// IDs (anything else). On query error all IDs route to the regular branch —
// matches the conservative fallback in isActiveWispID.
func partitionWispIDsPG(ctx context.Context, c pgxConn, ids []string) (wisps, regulars []string, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	wispSet := make(map[string]struct{}, len(ids))
	for i := 0; i < len(ids); i += deleteBatchSizePG {
		end := i + deleteBatchSizePG
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]
		args := make([]any, len(batch))
		for j, id := range batch {
			args[j] = id
		}
		ph := joinPlaceholders(1, len(batch))
		rows, qerr := c.Query(ctx, fmt.Sprintf("SELECT id FROM wisps WHERE id IN (%s)", ph), args...)
		if qerr != nil {
			return nil, ids, nil
		}
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr == nil {
				wispSet[id] = struct{}{}
			}
		}
		rows.Close()
		if rerr := rows.Err(); rerr != nil {
			return nil, nil, wrapErr("partition wisp ids", rerr)
		}
	}

	wisps = make([]string, 0, len(wispSet))
	regulars = make([]string, 0, len(ids)-len(wispSet))
	for _, id := range ids {
		if _, ok := wispSet[id]; ok {
			wisps = append(wisps, id)
		} else {
			regulars = append(regulars, id)
		}
	}
	return wisps, regulars, nil
}

// deleteWispsBatchedPG removes wisp IDs in ≤500 batches, each its own
// transaction. Matches the Dolt-side semantics in dolt/issues.go:404-410.
// Returns the total RowsAffected on the wisps table.
func deleteWispsBatchedPG(ctx context.Context, pool *pgxpool.Pool, wispIDs []string) (int, error) {
	total := 0
	for i := 0; i < len(wispIDs); i += wispDeleteBatchPG {
		end := i + wispDeleteBatchPG
		if end > len(wispIDs) {
			end = len(wispIDs)
		}
		n, err := deleteOneWispBatchPG(ctx, pool, wispIDs[i:end])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// deleteOneWispBatchPG runs the wisp-table cascade for one batch in a single
// transaction. Dependencies are deleted via TWO queries (issue_id and
// depends_on_id) rather than a single OR clause for index-plan stability —
// see the perf note in dolt/wisps.go:308-318.
func deleteOneWispBatchPG(ctx context.Context, pool *pgxpool.Pool, batch []string) (int, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, wrapErr("begin wisp delete batch", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	args := make([]any, len(batch))
	for i, id := range batch {
		args[i] = id
	}
	ph := joinPlaceholders(1, len(batch))

	if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM wisp_dependencies WHERE issue_id IN (%s)", ph), args...); err != nil {
		return 0, wrapErr("delete wisp_dependencies by issue_id", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM wisp_dependencies WHERE depends_on_id IN (%s)", ph), args...); err != nil {
		return 0, wrapErr("delete wisp_dependencies by depends_on_id", err)
	}
	for _, table := range []string{"wisp_events", "wisp_comments", "wisp_labels"} {
		q := fmt.Sprintf("DELETE FROM %s WHERE issue_id IN (%s)", table, ph)
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return 0, wrapErr(fmt.Sprintf("delete %s by issue_id", table), err)
		}
	}
	cmd, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM wisps WHERE id IN (%s)", ph), args...)
	if err != nil {
		return 0, wrapErr("delete wisps", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, wrapErr("commit wisp delete batch", err)
	}
	return int(cmd.RowsAffected()), nil
}

// deleteRegularIssuesInTxPG implements the regular-branch flow from
// be-ptzlki §3: expand the set (cascade/force/check-orphans), populate stats,
// and — unless dryRun — delete inbound dep edges then the issue rows.
// CASCADE handles labels/comments/events/snapshots/child_counters.
func deleteRegularIssuesInTxPG(ctx context.Context, tx pgx.Tx, ids []string, cascade, force, dryRun bool) (*types.DeleteIssuesResult, error) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	result := &types.DeleteIssuesResult{}

	expandedIDs := ids
	switch {
	case cascade:
		all, err := findAllDependentsRecursivePG(ctx, tx, ids)
		if err != nil {
			return nil, fmt.Errorf("find dependents: %w", err)
		}
		expandedIDs = make([]string, 0, len(all))
		for id := range all {
			expandedIDs = append(expandedIDs, id)
		}
	case !force:
		// Check for external dependents; error before any writes.
		for i := 0; i < len(ids); i += deleteBatchSizePG {
			end := i + deleteBatchSizePG
			if end > len(ids) {
				end = len(ids)
			}
			batch := ids[i:end]
			args := make([]any, len(batch))
			for j, id := range batch {
				args[j] = id
			}
			ph := joinPlaceholders(1, len(batch))
			rows, err := tx.Query(ctx,
				fmt.Sprintf("SELECT depends_on_id, issue_id FROM dependencies WHERE depends_on_id IN (%s)", ph),
				args...)
			if err != nil {
				return nil, wrapErr("check dependents", err)
			}
			externalBySource := make(map[string][]string)
			for rows.Next() {
				var depOnID, issueID string
				if scanErr := rows.Scan(&depOnID, &issueID); scanErr != nil {
					rows.Close()
					return nil, wrapErr("scan dependent", scanErr)
				}
				if !idSet[issueID] {
					externalBySource[depOnID] = append(externalBySource[depOnID], issueID)
				}
			}
			rows.Close()
			if rerr := rows.Err(); rerr != nil {
				return nil, wrapErr("iterate dependents", rerr)
			}
			for _, id := range batch {
				if deps, ok := externalBySource[id]; ok {
					result.OrphanedIssues = deps
					return result, fmt.Errorf("issue %s has dependents not in deletion set; use --cascade to delete them or --force to orphan them", id)
				}
			}
		}
	default:
		// Force mode: track orphans, proceed to delete.
		orphans, err := findExternalDependentsPG(ctx, tx, ids, idSet)
		if err != nil {
			return nil, fmt.Errorf("find external dependents: %w", err)
		}
		result.OrphanedIssues = orphans
	}

	// Stats.
	expandedIDSet := make(map[string]bool, len(expandedIDs))
	for _, id := range expandedIDs {
		expandedIDSet[id] = true
	}

	var depsCount, labelsCount, eventsCount int
	for i := 0; i < len(expandedIDs); i += deleteBatchSizePG {
		end := i + deleteBatchSizePG
		if end > len(expandedIDs) {
			end = len(expandedIDs)
		}
		batch := expandedIDs[i:end]
		args := make([]any, len(batch))
		for j, id := range batch {
			args[j] = id
		}
		ph := joinPlaceholders(1, len(batch))

		var n int
		if err := tx.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM dependencies WHERE issue_id IN (%s)", ph),
			args...).Scan(&n); err != nil {
			return nil, wrapErr("count dependencies", err)
		}
		depsCount += n
		if err := tx.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM labels WHERE issue_id IN (%s)", ph),
			args...).Scan(&n); err != nil {
			return nil, wrapErr("count labels", err)
		}
		labelsCount += n
		if err := tx.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM events WHERE issue_id IN (%s)", ph),
			args...).Scan(&n); err != nil {
			return nil, wrapErr("count events", err)
		}
		eventsCount += n
	}

	// Inbound deps from outside the set count toward DependenciesCount.
	for i := 0; i < len(expandedIDs); i += deleteBatchSizePG {
		end := i + deleteBatchSizePG
		if end > len(expandedIDs) {
			end = len(expandedIDs)
		}
		batch := expandedIDs[i:end]
		args := make([]any, len(batch))
		for j, id := range batch {
			args[j] = id
		}
		ph := joinPlaceholders(1, len(batch))
		rows, err := tx.Query(ctx,
			fmt.Sprintf("SELECT issue_id FROM dependencies WHERE depends_on_id IN (%s)", ph),
			args...)
		if err != nil {
			return nil, wrapErr("count inbound dependencies", err)
		}
		for rows.Next() {
			var issID string
			if scanErr := rows.Scan(&issID); scanErr != nil {
				rows.Close()
				return nil, wrapErr("scan inbound dependency", scanErr)
			}
			if !expandedIDSet[issID] {
				depsCount++
			}
		}
		rows.Close()
		if rerr := rows.Err(); rerr != nil {
			return nil, wrapErr("iterate inbound dependencies", rerr)
		}
	}

	result.DependenciesCount = depsCount
	result.LabelsCount = labelsCount
	result.EventsCount = eventsCount
	result.DeletedCount = len(expandedIDs)

	if dryRun {
		return result, nil
	}

	totalDeleted := 0
	for i := 0; i < len(expandedIDs); i += deleteBatchSizePG {
		end := i + deleteBatchSizePG
		if end > len(expandedIDs) {
			end = len(expandedIDs)
		}
		batch := expandedIDs[i:end]
		args := make([]any, len(batch))
		for j, id := range batch {
			args[j] = id
		}
		ph := joinPlaceholders(1, len(batch))

		if _, err := tx.Exec(ctx,
			fmt.Sprintf("DELETE FROM dependencies WHERE depends_on_id IN (%s)", ph),
			args...); err != nil {
			return nil, wrapErr("delete inbound dependencies", err)
		}
		cmd, err := tx.Exec(ctx,
			fmt.Sprintf("DELETE FROM issues WHERE id IN (%s)", ph),
			args...)
		if err != nil {
			return nil, wrapErr("delete issues", err)
		}
		totalDeleted += int(cmd.RowsAffected())
	}
	result.DeletedCount = totalDeleted
	return result, nil
}

// findAllDependentsRecursivePG walks the dependencies graph breadth-first
// starting from ids, returning the seed set plus every transitive dependent.
// Bounded by maxRecursiveResultsPG to terminate on cycles.
func findAllDependentsRecursivePG(ctx context.Context, c pgxConn, ids []string) (map[string]bool, error) {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	frontier := append([]string(nil), ids...)

	for len(frontier) > 0 && len(seen) < maxRecursiveResultsPG {
		next := frontier
		frontier = nil
		for i := 0; i < len(next); i += deleteBatchSizePG {
			end := i + deleteBatchSizePG
			if end > len(next) {
				end = len(next)
			}
			batch := next[i:end]
			args := make([]any, len(batch))
			for j, id := range batch {
				args[j] = id
			}
			ph := joinPlaceholders(1, len(batch))
			rows, err := c.Query(ctx,
				fmt.Sprintf("SELECT issue_id FROM dependencies WHERE depends_on_id IN (%s)", ph),
				args...)
			if err != nil {
				return nil, wrapErr("recursive dependents query", err)
			}
			for rows.Next() {
				var id string
				if scanErr := rows.Scan(&id); scanErr != nil {
					rows.Close()
					return nil, wrapErr("scan recursive dependent", scanErr)
				}
				if !seen[id] {
					seen[id] = true
					frontier = append(frontier, id)
					if len(seen) >= maxRecursiveResultsPG {
						break
					}
				}
			}
			rows.Close()
			if rerr := rows.Err(); rerr != nil {
				return nil, wrapErr("iterate recursive dependents", rerr)
			}
		}
	}
	return seen, nil
}

// findExternalDependentsPG returns the set of issue IDs that depend on any
// of ids but are NOT themselves in idSet. Used by the force-mode path to
// populate OrphanedIssues for reporting (cascade is the alternative path).
func findExternalDependentsPG(ctx context.Context, c pgxConn, ids []string, idSet map[string]bool) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for i := 0; i < len(ids); i += deleteBatchSizePG {
		end := i + deleteBatchSizePG
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]
		args := make([]any, len(batch))
		for j, id := range batch {
			args[j] = id
		}
		ph := joinPlaceholders(1, len(batch))
		rows, err := c.Query(ctx,
			fmt.Sprintf("SELECT DISTINCT issue_id FROM dependencies WHERE depends_on_id IN (%s)", ph),
			args...)
		if err != nil {
			return nil, wrapErr("find external dependents", err)
		}
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr != nil {
				rows.Close()
				return nil, wrapErr("scan external dependent", scanErr)
			}
			if !idSet[id] && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		rows.Close()
		if rerr := rows.Err(); rerr != nil {
			return nil, wrapErr("iterate external dependents", rerr)
		}
	}
	return out, nil
}

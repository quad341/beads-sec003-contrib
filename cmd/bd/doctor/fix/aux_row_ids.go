package fix

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steveyegge/beads/internal/storage/schema"
)

// AuxRowIDs repairs events/comments/issue_snapshots/compaction_snapshots rows
// whose id has drifted from its content-derived target
// (internal/storage/rowid) — see schema.RekeyAuxRowIDs. Unlike
// DependencyKeys' repair, this only ever re-keys rows (UPDATE); it never
// deletes one, since every aux row is a real event, comment or snapshot
// regardless of what its id currently is.
// If verbose is true, prints each repaired table; otherwise shows only a summary.
func AuxRowIDs(path string, verbose bool) error {
	beadsDir, err := resolvedWorkspaceBeadsDir(path)
	if err != nil {
		return err
	}

	db, cfg, err := openDoltDB(beadsDir)
	if err != nil {
		fmt.Printf("  Aux row id fix skipped (%v)\n", err)
		return nil
	}
	defer db.Close()

	if skip, err := guardFixTarget("Aux row id fix", db, beadsDir, cfg); skip {
		return err
	}

	return repairAuxRowIDs(context.Background(), db, verbose)
}

// repairAuxRowIDs scans and repairs aux row id drift on an open connection.
// Split from AuxRowIDs so the repair logic is testable against an existing
// store handle.
func repairAuxRowIDs(ctx context.Context, db *sql.DB, verbose bool) error {
	anomalies, err := schema.ScanAuxRowIDs(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to scan aux row ids: %w", err)
	}
	if len(anomalies) == 0 {
		fmt.Println("  No aux row id anomalies to fix")
		return nil
	}

	// Explicit transaction so writes persist when @@autocommit is OFF (e.g.
	// Dolt server started with --no-auto-commit) — mirrors repairDependencyKeys.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	if _, err := schema.RekeyAuxRowIDs(ctx, tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to re-key aux row ids: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit aux row id repairs: %w", err)
	}

	var repairedTables []string
	var total int
	for _, a := range anomalies {
		total += a.Count
		repairedTables = append(repairedTables, a.Table)
		if verbose {
			fmt.Printf("  Re-keyed %s: %d row(s)\n", a.Table, a.Count)
		}
	}

	// Commit changes in Dolt, staging only the repaired tables so an unrelated
	// dirty working set is not swept under this message. Best effort: commit
	// advisory; repair already applied.
	for _, table := range repairedTables {
		_, _ = db.Exec("CALL DOLT_ADD(?)", table)
	}
	_, _ = db.Exec("CALL DOLT_COMMIT('-m', 'doctor: re-key aux row ids to content-derived values')")

	fmt.Printf("  Fixed aux row ids: %d row(s) across %d table(s)\n", total, len(repairedTables))
	return nil
}

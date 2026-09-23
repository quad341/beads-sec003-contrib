package schema

import (
	"context"
	"fmt"
)

// AuxRowIDAnomaly names one auxRekeyTables table holding rows whose id has
// drifted from its content-derived target (internal/storage/rowid) — e.g. a
// clone that ran the 0037/bd-ri8bd backfills independently of its peers, or
// whose backfill was interrupted mid-pass (bd-578h9). It is comparable with
// ==, so tests can compare a scan result with slices.Equal.
type AuxRowIDAnomaly struct {
	Table string
	Count int
}

// ScanAuxRowIDs reports, for every auxRekeyTables entry, how many rows would
// be re-keyed by RekeyAuxRowIDs. It is read-only and unconditional — unlike
// the migration-time rekeyAuxRowIDs pass, it is not gated by a
// once-per-clone marker or crash sentinel, so it is safe to call on demand,
// repeatedly, from bd doctor. Tables with nothing to do are omitted rather
// than listed at zero, matching cmd/bd/doctor/fix.ScanDependencyKeys'
// convention for the same class of check.
//
// Tables are scanned in auxRekeyTables' fixed order and the scan stops at the
// first table error, leaving later tables unprobed — a caller that needs a
// best-effort scan across a partially unreadable database should call
// planAuxRowTableRekey per table directly instead.
func ScanAuxRowIDs(ctx context.Context, db DBConn) ([]AuxRowIDAnomaly, error) {
	var anomalies []AuxRowIDAnomaly
	for _, t := range auxRekeyTables {
		todo, err := planAuxRowTableRekey(ctx, db, t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.name, err)
		}
		if len(todo) > 0 {
			anomalies = append(anomalies, AuxRowIDAnomaly{Table: t.name, Count: len(todo)})
		}
	}
	return anomalies, nil
}

// RekeyAuxRowIDs repairs every auxRekeyTables entry's id drift in place. It
// only re-keys rows (UPDATE); unlike cmd/bd/doctor/fix's dependency-key
// repair it never deletes a row — every aux row is a real event, comment or
// snapshot regardless of what its id currently is, so there is nothing here
// that is ever correct to remove. It reports whether any row was changed,
// and is idempotent: a clone with nothing to repair issues no writes. Like
// ScanAuxRowIDs it is unconditional, never gated by the migration-time
// markers or sentinels that guard rekeyAuxRowIDs, and it stops at the first
// table error rather than repairing the remaining tables.
func RekeyAuxRowIDs(ctx context.Context, db DBConn) (bool, error) {
	var wrote bool
	for _, t := range auxRekeyTables {
		w, err := rekeyAuxRowTable(ctx, db, t)
		wrote = wrote || w
		if err != nil {
			return wrote, fmt.Errorf("%s: %w", t.name, err)
		}
	}
	return wrote, nil
}

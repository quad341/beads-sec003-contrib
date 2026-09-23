package schema

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/steveyegge/beads/internal/storage/rowid"
)

// issueSnapshotsTable and compactionSnapshotsTable name the two auxRekeyTables
// entries aux_row_id_backfill_test.go's commentsTable and
// aux_row_id_drift_test.go's eventsTable don't already cover.
var issueSnapshotsTable = auxRekeyTables[2]
var compactionSnapshotsTable = auxRekeyTables[3]

// expectAuxTableSelect returns the ExpectedQuery for table t's full
// rekeyAuxRowTable SELECT — the generic, all-four-tables counterpart to
// aux_row_id_backfill_test.go's expectCommentsSelect and
// aux_row_id_drift_test.go's expectEventsSelect.
func expectAuxTableSelect(mock sqlmock.Sqlmock, t auxRekeyTable) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(
		fmt.Sprintf("SELECT id, %s FROM %s", t.columns, t.name)))
}

// expectAuxTableEmpty sets up a clean scan of table t: present, zero rows —
// filler for the auxRekeyTables entries not under test in a given case. The
// column labels are arbitrary (rekeyAuxRowTable scans positionally), so only
// their count needs to match t.columns.
func expectAuxTableEmpty(mock sqlmock.Sqlmock, t auxRekeyTable) {
	expectColumnExists(mock, true)
	n := strings.Count(t.columns, ",") + 1
	cols := make([]string, n+1)
	cols[0] = "id"
	for i := 1; i <= n; i++ {
		cols[i] = fmt.Sprintf("f%d", i)
	}
	expectAuxTableSelect(mock, t).WillReturnRows(sqlmock.NewRows(cols))
}

// TestScanAuxRowIDsReportsPerTableCounts verifies the exit_contract's
// per-table count requirement: a table holding random (pre-rekey) ids is
// named with an accurate count of divergent rows, in auxRekeyTables' own
// iteration order (events, comments, issue_snapshots, compaction_snapshots),
// and clean tables are omitted entirely rather than listed at zero — bd
// doctor only wants to hear about tables that actually need attention.
func TestScanAuxRowIDsReportsPerTableCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectAuxTableEmpty(mock, eventsTable)
	expectColumnExists(mock, true)
	expectCommentsSelect(mock).
		WillReturnRows(sqlmock.NewRows([]string{"id", "issue_id", "author", "text", "created_at"}).
			AddRow("random-a", "bd-1", "steve", "first", "2026-06-09 12:00:00").
			AddRow("random-b", "bd-1", "steve", "second", "2026-06-09 12:00:01"))
	expectAuxTableEmpty(mock, issueSnapshotsTable)
	expectAuxTableEmpty(mock, compactionSnapshotsTable)

	got, err := ScanAuxRowIDs(context.Background(), db)
	if err != nil {
		t.Fatalf("ScanAuxRowIDs: %v", err)
	}
	want := []AuxRowIDAnomaly{{Table: "comments", Count: 2}}
	if !slices.Equal(got, want) {
		t.Errorf("ScanAuxRowIDs = %+v, want %+v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestScanAuxRowIDsCleanCloneReportsNoAnomalies is the unit-level proxy for
// the exit_contract's "zero false positives on a healthy clone": tables that
// are empty or already hold deterministic ids must report no anomalies at
// all, not an empty-but-present entry.
func TestScanAuxRowIDsCleanCloneReportsNoAnomalies(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	digest := commentDigest("bd-1", "steve", "hello", "2026-06-09 12:00:00")
	expectAuxTableEmpty(mock, eventsTable)
	expectColumnExists(mock, true)
	expectCommentsSelect(mock).
		WillReturnRows(sqlmock.NewRows([]string{"id", "issue_id", "author", "text", "created_at"}).
			AddRow(rowid.New("comments", 0, digest), "bd-1", "steve", "hello", "2026-06-09 12:00:00"))
	expectAuxTableEmpty(mock, issueSnapshotsTable)
	expectAuxTableEmpty(mock, compactionSnapshotsTable)

	got, err := ScanAuxRowIDs(context.Background(), db)
	if err != nil {
		t.Fatalf("ScanAuxRowIDs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanAuxRowIDs = %+v, want no anomalies", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestScanAuxRowIDsSkipsMissingTable verifies a table/column absent (older or
// partial schema) is treated like rekeyAuxRowTable treats it: silently
// skipped, not reported as an anomaly and not an error.
func TestScanAuxRowIDsSkipsMissingTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectColumnExists(mock, false) // events: id column absent
	expectAuxTableEmpty(mock, commentsTable)
	expectAuxTableEmpty(mock, issueSnapshotsTable)
	expectAuxTableEmpty(mock, compactionSnapshotsTable)

	got, err := ScanAuxRowIDs(context.Background(), db)
	if err != nil {
		t.Fatalf("ScanAuxRowIDs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanAuxRowIDs = %+v, want no anomalies", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestScanAuxRowIDsPropagatesTableScanError verifies a table scan failure
// aborts the whole scan immediately (fail fast, mirroring
// rekeyAuxRowIDsPending's non-drift-error handling) rather than silently
// under-reporting: no later table in auxRekeyTables may be probed once an
// earlier one fails, which is proven here by registering expectations for
// events only.
func TestScanAuxRowIDsPropagatesTableScanError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectColumnExists(mock, true)
	expectAuxTableSelect(mock, eventsTable).WillReturnError(errors.New("boom"))

	got, err := ScanAuxRowIDs(context.Background(), db)
	if err == nil {
		t.Fatal("expected the table scan failure to propagate")
	}
	if !strings.Contains(err.Error(), "events") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %q, want it to name the failing table and wrap the cause", err.Error())
	}
	if got != nil {
		t.Errorf("ScanAuxRowIDs = %+v on error, want nil", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestRekeyAuxRowIDsAppliesUpdatesAcrossTables verifies the doctor --fix
// entry point runs the real rewrite (not a dry-run count) across every
// auxRekeyTables entry, unconditionally — unlike the migration-time
// rekeyAuxRowIDs pass, it is never gated by a marker or sentinel, so this
// registers no ignored_schema_migrations/local_metadata expectations at all;
// any such call would be unexpected and fail the mock.
func TestRekeyAuxRowIDsAppliesUpdatesAcrossTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectAuxTableEmpty(mock, eventsTable)
	expectColumnExists(mock, true)
	expectCommentsSelect(mock).
		WillReturnRows(sqlmock.NewRows([]string{"id", "issue_id", "author", "text", "created_at"}).
			AddRow("random-a", "bd-1", "steve", "first", "2026-06-09 12:00:00"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE comments SET id = ? WHERE id = ?")).
		WithArgs(rowid.New("comments", 0, commentDigest("bd-1", "steve", "first", "2026-06-09 12:00:00")), "random-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectAuxTableEmpty(mock, issueSnapshotsTable)
	expectAuxTableEmpty(mock, compactionSnapshotsTable)

	wrote, err := RekeyAuxRowIDs(context.Background(), db)
	if err != nil {
		t.Fatalf("RekeyAuxRowIDs: %v", err)
	}
	if !wrote {
		t.Error("expected wrote=true when a row was re-keyed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestRekeyAuxRowIDsIdempotentNoWrites covers the exit_contract's idempotence
// requirement directly: run against a clone with nothing to re-key, zero
// UPDATE statements may be issued (there is no ExpectExec registered at all,
// so any UPDATE call would be unexpected and fail the mock).
func TestRekeyAuxRowIDsIdempotentNoWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectAuxTableEmpty(mock, eventsTable)
	expectAuxTableEmpty(mock, commentsTable)
	expectAuxTableEmpty(mock, issueSnapshotsTable)
	expectAuxTableEmpty(mock, compactionSnapshotsTable)

	wrote, err := RekeyAuxRowIDs(context.Background(), db)
	if err != nil {
		t.Fatalf("RekeyAuxRowIDs: %v", err)
	}
	if wrote {
		t.Error("expected wrote=false when nothing needed re-keying")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestRekeyAuxRowIDsPropagatesTableError verifies a table failure aborts the
// fix immediately rather than silently repairing only some tables.
func TestRekeyAuxRowIDsPropagatesTableError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectColumnExists(mock, true)
	expectAuxTableSelect(mock, eventsTable).WillReturnError(errors.New("boom"))

	wrote, err := RekeyAuxRowIDs(context.Background(), db)
	if err == nil {
		t.Fatal("expected the table failure to propagate")
	}
	if !strings.Contains(err.Error(), "events") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %q, want it to name the failing table and wrap the cause", err.Error())
	}
	if wrote {
		t.Error("expected wrote=false when the first table fails before any UPDATE")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

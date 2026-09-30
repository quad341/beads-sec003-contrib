package issueops

import (
	"context"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// These tests pin the SQL contract of RemoveVersionInTx: the statements it
// issues and the order it issues them in. What those statements DO to real
// rows (first removal wins, only the named row changes, nothing is deleted)
// is pinned against real Dolt engines in the dolt and embeddeddolt legs'
// TestVersionRemoval* tests; this file covers what a real engine cannot show
// from a single-threaded test -- that the write is guarded against a
// concurrent removal, that a bad reason never reaches the database, and that
// removal is an UPDATE rather than a DELETE.

const (
	// versionRemovalProbeSQL is the read that tells "no such version" apart
	// from "already removed" without depending on any driver's RowsAffected
	// semantics (changed rows vs matched rows differ across drivers).
	versionRemovalProbeSQL = `SELECT removed_at IS NOT NULL FROM issue_versions WHERE issue_id = \? AND revision = \?`

	// versionRemovalUpdateSQL is the write. The trailing removed_at IS NULL
	// is what keeps "first removal wins" true when a second transaction
	// removes the same version between the probe above and this UPDATE.
	versionRemovalUpdateSQL = `UPDATE issue_versions SET removed_at = \?, removed_reason = \? WHERE issue_id = \? AND revision = \? AND removed_at IS NULL`
)

func TestVersionRemovalStampsTheNamedRowWithAGuardedUpdate(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{
		VersionRemovalReasonRetention,
		VersionRemovalReasonErasure,
		VersionRemovalReasonReorganization,
	} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer func() { _ = db.Close() }()

			mock.ExpectQuery(versionRemovalProbeSQL).
				WithArgs("bd-1", int64(3)).
				WillReturnRows(sqlmock.NewRows([]string{"removed"}).AddRow(false))
			mock.ExpectExec(versionRemovalUpdateSQL).
				WithArgs(sqlmock.AnyArg(), reason, "bd-1", int64(3)).
				WillReturnResult(sqlmock.NewResult(0, 1))

			if err := RemoveVersionInTx(context.Background(), db, "bd-1", 3, reason); err != nil {
				t.Fatalf("RemoveVersionInTx(reason=%q) = %v, want nil", reason, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("statements issued did not match the contract: %v", err)
			}
		})
	}
}

func TestVersionRemovalOfAnAbsentVersionReportsNotFoundAndWritesNothing(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	// The probe finds no row; no UPDATE is expected, so any write attempt
	// fails the call with sqlmock's "was not expected" error instead.
	mock.ExpectQuery(versionRemovalProbeSQL).
		WithArgs("bd-1", int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"removed"}))

	err = RemoveVersionInTx(context.Background(), db, "bd-1", 99, VersionRemovalReasonReorganization)
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("RemoveVersionInTx(absent version) = %v, want an error wrapping ErrVersionNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("statements issued did not match the contract: %v", err)
	}
}

func TestVersionRemovalRejectsAReasonOutsideTheVocabularyBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	// No expectations are registered: a bad reason must fail before any
	// statement is issued, or the removal marker could be written with a
	// value no resolver knows how to read back.
	const bogus = "because-i-said-so"
	err = RemoveVersionInTx(context.Background(), db, "bd-1", 1, bogus)
	if err == nil {
		t.Fatal("RemoveVersionInTx with an unknown reason = nil, want an error")
	}
	if !strings.Contains(err.Error(), bogus) {
		t.Errorf("error %q does not name the rejected reason %q", err, bogus)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("a rejected reason still reached the database: %v", err)
	}
}

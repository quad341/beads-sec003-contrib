package testutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestSetupSharedTestDB_RefusesAmbientPortMismatch guards the same class of
// bug as TestDoltContainerStartSites_SetBeadsServerPortEnv: if something
// upstream in the resolution chain ever passes a port that disagrees with
// the ambient BEADS_DOLT_SERVER_PORT, SetupSharedTestDB must refuse rather
// than silently create a database against whichever server
// BEADS_DOLT_SERVER_PORT happens to point at. This check must not require a
// real Dolt server: it has to fire before any connection is attempted, the
// same way the existing production-port (3307) firewall does.
func TestSetupSharedTestDB_RefusesAmbientPortMismatch(t *testing.T) {
	t.Setenv("BEADS_DOLT_SERVER_PORT", "19191") // decoy ambient shared server

	db, err := SetupSharedTestDB(29292, "irrelevant_db") // disagrees with ambient port above
	if db != nil {
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("SetupSharedTestDB: expected refusal when port disagrees with ambient BEADS_DOLT_SERVER_PORT, got nil error")
	}
	if !strings.Contains(err.Error(), "BEADS_DOLT_SERVER_PORT") {
		t.Errorf("SetupSharedTestDB error = %q, want it to name the ambient BEADS_DOLT_SERVER_PORT disagreement", err.Error())
	}
}

// TestSetupSharedTestDB_RefusesProductionPort guards the production-port
// firewall: SetupSharedTestDB must refuse port 3307 before running any DDL
// (Clown Shows #12-#18 were test databases leaking onto a production server).
// Like the ambient-port guard above, it has to fire without a real Dolt server.
//
// Were the firewall to regress, SetupSharedTestDB would go on to run CREATE
// DATABASE against whatever answers on 127.0.0.1:3307, so the test declines to
// run at all while something is listening there.
func TestSetupSharedTestDB_RefusesProductionPort(t *testing.T) {
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:3307", 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Skip("something is listening on 127.0.0.1:3307; not risking a live server if the firewall regressed")
	}
	// Clear the ambient port so the ambient-port guard cannot be what refuses,
	// masking this guard regressing.
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")

	db, err := SetupSharedTestDB(3307, "should_never_be_created")
	if db != nil {
		_ = db.Close()
		t.Error("SetupSharedTestDB on the production port returned a *sql.DB; it must return nil when refusing")
	}
	if err == nil {
		t.Fatal("SetupSharedTestDB: expected refusal on production port 3307, got nil error")
	}
	// Both words: a bare "connection refused" from a broken firewall must not
	// satisfy this.
	if !strings.Contains(err.Error(), "REFUSED") || !strings.Contains(err.Error(), "production") {
		t.Errorf("SetupSharedTestDB error = %q, want the production-port REFUSED message", err.Error())
	}
}

// scriptedExec is a doltBranchSQL whose ExecContext result is scripted per
// attempt. It drives the visibility wait through a catalog race
// deterministically: a live server cannot be made to lag its own catalog on
// demand, and on a healthy one the race never shows.
type scriptedExec struct {
	respond func(attempt int) error // attempt is 1-based; nil means the statement succeeds
	queries []string
}

func (s *scriptedExec) ExecContext(ctx context.Context, query string, _ ...interface{}) (sql.Result, error) {
	// Like database/sql, refuse to run once the context is done.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.queries = append(s.queries, query)
	if err := s.respond(len(s.queries)); err != nil {
		return nil, err
	}
	return driver.RowsAffected(0), nil
}

func (s *scriptedExec) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("scriptedExec: QueryContext is not scripted")
}

// catalogMiss is what a Dolt server answers to USE <dbName> while its catalog
// has not registered the database yet, as observed against dolt 2.2.0:
// "Error 1049 (HY000): database not found: <dbName>".
func catalogMiss(dbName string) error {
	return &mysql.MySQLError{Number: 1049, SQLState: [5]byte{'H', 'Y', '0', '0', '0'}, Message: "database not found: " + dbName}
}

// TestSetupSharedTestDB_VisibilityWait_RetriesUntilVisible pins the be-s9d fix:
// after CREATE DATABASE, SetupSharedTestDB must poll USE until the new database
// is visible instead of returning into a sibling connection that then fails
// with "database not found".
func TestSetupSharedTestDB_VisibilityWait_RetriesUntilVisible(t *testing.T) {
	const dbName = "shared_catalog_race"
	exec := &scriptedExec{respond: func(attempt int) error {
		if attempt <= 3 {
			return catalogMiss(dbName)
		}
		return nil
	}}

	if err := waitForDatabaseVisible(context.Background(), exec, dbName); err != nil {
		t.Fatalf("waitForDatabaseVisible: %v", err)
	}
	if len(exec.queries) != 4 {
		t.Errorf("USE attempts = %d, want 4 (three catalog misses, then visible)", len(exec.queries))
	}
	for i, q := range exec.queries {
		if want := "USE `" + dbName + "`"; q != want {
			t.Errorf("attempt %d ran %q, want %q", i+1, q, want)
		}
	}
}

// TestSetupSharedTestDB_VisibilityWait_RetriesEveryCatalogMissWording covers
// each shape "the database is not there yet" reaches the wait in: Dolt's
// wording, stock MySQL's, and either flattened to text by a wrapper that lost
// the *mysql.MySQLError type. It mirrors uow.isDatabaseNotFoundError, which
// testutil cannot import (uow's tests import testutil).
func TestSetupSharedTestDB_VisibilityWait_RetriesEveryCatalogMissWording(t *testing.T) {
	const dbName = "shared_catalog_race"
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"dolt typed 1049", catalogMiss(dbName)},
		{"mysql typed 1049", &mysql.MySQLError{Number: 1049, SQLState: [5]byte{'4', '2', '0', '0', '0'}, Message: "Unknown database '" + dbName + "'"}},
		// The number alone must be enough, so a server rewording its message
		// does not silently turn a catalog miss into a hard failure.
		{"typed 1049 with unrecognised wording", &mysql.MySQLError{Number: 1049, SQLState: [5]byte{'H', 'Y', '0', '0', '0'}, Message: "no such schema: " + dbName}},
		{"dolt text only", fmt.Errorf("use %s: %s", dbName, catalogMiss(dbName))},
		{"mysql text only", errors.New("Error 1049 (42000): Unknown database '" + dbName + "'")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := &scriptedExec{respond: func(attempt int) error {
				if attempt == 1 {
					return tc.err
				}
				return nil
			}}

			if err := waitForDatabaseVisible(context.Background(), exec, dbName); err != nil {
				t.Fatalf("waitForDatabaseVisible: %v", err)
			}
			if len(exec.queries) != 2 {
				t.Errorf("USE attempts = %d, want 2 (one miss, then visible)", len(exec.queries))
			}
		})
	}
}

// TestSetupSharedTestDB_VisibilityWait_PermanentErrorsAreNotRetried keeps the
// wait from papering over real failures: anything that is not "database not
// visible yet" must surface at once, not after a 10s poll.
func TestSetupSharedTestDB_VisibilityWait_PermanentErrorsAreNotRetried(t *testing.T) {
	const dbName = "shared_catalog_race"
	for _, tc := range []struct {
		name string
		err  error
	}{
		// A server hides a database from a credential that was not granted it by
		// answering access-denied, so this says nothing about the catalog and
		// waiting cannot fix it.
		{"access denied", &mysql.MySQLError{Number: 1044, SQLState: [5]byte{'4', '2', '0', '0', '0'}, Message: "Access denied for user 'root'@'%' to database '" + dbName + "'"}},
		{"connection refused", errors.New("dial tcp 127.0.0.1:1: connect: connection refused")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := &scriptedExec{respond: func(int) error { return tc.err }}

			err := waitForDatabaseVisible(context.Background(), exec, dbName)
			if !errors.Is(err, tc.err) {
				t.Fatalf("waitForDatabaseVisible = %v, want it to wrap %v", err, tc.err)
			}
			if len(exec.queries) != 1 {
				t.Errorf("USE attempts = %d, want 1: a permanent error must fail fast, not be retried", len(exec.queries))
			}
		})
	}
}

// TestSetupSharedTestDB_VisibilityWait_DeadlineMidStatementKeepsLastMiss covers
// the deadline landing while a USE is in flight, where database/sql reports the
// cancellation rather than the server's answer: the error must still carry the
// last "database not found" so the failure says what was actually wrong.
func TestSetupSharedTestDB_VisibilityWait_DeadlineMidStatementKeepsLastMiss(t *testing.T) {
	const dbName = "shared_catalog_race"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec := &scriptedExec{respond: func(attempt int) error {
		if attempt < 3 {
			return catalogMiss(dbName)
		}
		cancel()
		return context.Canceled
	}}

	err := waitForDatabaseVisible(ctx, exec, dbName)

	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1049 {
		t.Fatalf("waitForDatabaseVisible = %v, want it to wrap the server's last 1049, not only the cancellation", err)
	}
}

// TestSetupSharedTestDB_VisibilityWait_GivesUpWhenNeverVisible bounds the wait:
// a database that never becomes visible must produce an error naming it and
// the server's last answer at the caller's deadline, never a hang.
func TestSetupSharedTestDB_VisibilityWait_GivesUpWhenNeverVisible(t *testing.T) {
	const dbName = "shared_catalog_race"
	exec := &scriptedExec{respond: func(int) error { return catalogMiss(dbName) }}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	err := waitForDatabaseVisible(ctx, exec, dbName)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitForDatabaseVisible returned nil for a database that never became visible")
	}
	if elapsed > 5*time.Second {
		t.Errorf("waitForDatabaseVisible took %s; it must stop at its context deadline", elapsed)
	}
	if len(exec.queries) < 2 {
		t.Errorf("USE attempts = %d, want the wait to keep polling until its deadline", len(exec.queries))
	}
	if !strings.Contains(err.Error(), dbName) {
		t.Errorf("error %q does not name the database", err)
	}
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1049 {
		t.Errorf("error %q does not wrap the server's last 1049; a timeout must say what the server kept answering", err)
	}
}

// TestCreateSharedDatabase_CreatesThenWaitsForVisibility pins how the be-s9d
// fix is wired in, which the waitForDatabaseVisible tests above cannot: they
// call the wait directly, so a createSharedDatabase that stopped calling it
// would still pass them, and a healthy server never shows the race that a
// real-server test would need. The database must be created first, and the
// helper must not return until USE has stopped reporting a catalog miss.
func TestCreateSharedDatabase_CreatesThenWaitsForVisibility(t *testing.T) {
	const dbName = "shared_catalog_race"
	const misses = 3
	exec := &scriptedExec{respond: func(attempt int) error {
		// Attempt 1 is the CREATE; the USEs after it miss until the catalog
		// has caught up.
		if attempt > 1 && attempt <= 1+misses {
			return catalogMiss(dbName)
		}
		return nil
	}}

	if err := createSharedDatabase(context.Background(), exec, dbName); err != nil {
		t.Fatalf("createSharedDatabase: %v", err)
	}

	want := []string{"CREATE DATABASE IF NOT EXISTS `" + dbName + "`"}
	want = append(want, slices.Repeat([]string{"USE `" + dbName + "`"}, misses+1)...)
	if !slices.Equal(exec.queries, want) {
		t.Errorf("statements = %q\nwant         %q\n(CREATE first, then USE until its %d catalog misses clear)", exec.queries, want, misses)
	}
}

// TestCreateSharedDatabase_ToleratesDatabaseAlreadyExisting covers a database
// left by an earlier setup call: Dolt may answer CREATE DATABASE IF NOT EXISTS
// with error 1007 anyway. That is not a setup failure, and the wait must still
// run, since the database may be as invisible to this connection as a fresh one.
func TestCreateSharedDatabase_ToleratesDatabaseAlreadyExisting(t *testing.T) {
	const dbName = "shared_catalog_race"
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"database exists wording", errors.New("can't create database " + dbName + "; database exists")},
		// The number alone must be enough, so a server rewording its message does
		// not silently turn "already there" into a hard failure.
		{"typed 1007 with unrecognised wording", &mysql.MySQLError{Number: 1007, SQLState: [5]byte{'H', 'Y', '0', '0', '0'}, Message: "can't create schema " + dbName + "; schema exists"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := &scriptedExec{respond: func(attempt int) error {
				if attempt == 1 {
					return tc.err // the CREATE
				}
				return nil
			}}

			if err := createSharedDatabase(context.Background(), exec, dbName); err != nil {
				t.Fatalf("createSharedDatabase: %v", err)
			}
			if len(exec.queries) != 2 || !strings.HasPrefix(exec.queries[1], "USE ") {
				t.Errorf("statements = %q, want the CREATE followed by a USE: an existing database still gets the visibility wait", exec.queries)
			}
		})
	}
}

// TestCreateSharedDatabase_CreateFailureIsNotWaitedOut keeps the wait from
// papering over a CREATE that really failed: the error must surface at once,
// not after a poll for a database that was never made.
func TestCreateSharedDatabase_CreateFailureIsNotWaitedOut(t *testing.T) {
	const dbName = "shared_catalog_race"
	createErr := &mysql.MySQLError{Number: 1044, SQLState: [5]byte{'4', '2', '0', '0', '0'}, Message: "Access denied for user 'root'@'%' to database '" + dbName + "'"}
	exec := &scriptedExec{respond: func(int) error { return createErr }}

	err := createSharedDatabase(context.Background(), exec, dbName)

	if !errors.Is(err, createErr) {
		t.Fatalf("createSharedDatabase = %v, want it to wrap %v", err, createErr)
	}
	if !strings.Contains(err.Error(), "create database") {
		t.Errorf("error %q does not say the CREATE failed", err)
	}
	if len(exec.queries) != 1 {
		t.Errorf("statements = %q, want only the CREATE: a failed CREATE must not go on to poll USE", exec.queries)
	}
}

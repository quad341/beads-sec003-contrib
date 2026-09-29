package dolt

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/steveyegge/beads/internal/storage/doltutil"
	"github.com/steveyegge/beads/internal/testutil"
)

// Tests for the contract testutil.SetupSharedTestDB gives this package's
// harness (be-s9d): once it returns, the database it created is visible to a
// brand-new session, so the next dolt.New / initSharedSchema against it cannot
// fail with "Error 1049 (HY000): database not found: <name>".
//
// On a healthy local server the race behind that failure does not reproduce, so
// these tests guard the end-to-end contract rather than detect the race. The
// wait that closes it is pinned deterministically in internal/testutil, where a
// scripted server answer stands in for the lagging catalog.

// setupSharedTestDB runs SetupSharedTestDB for a fresh, uniquely named
// database and registers its cleanup.
func setupSharedTestDB(t *testing.T) (dbName string, conn *sql.DB) {
	t.Helper()
	dbName = uniqueTestDBName(t)
	conn, err := testutil.SetupSharedTestDB(testServerPort, dbName)
	if err != nil {
		t.Fatalf("SetupSharedTestDB(%q): %v", dbName, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	t.Cleanup(func() { dropTestDatabase(t, testServerPort, dbName) })
	return dbName, conn
}

// TestSetupSharedTestDB_FreshRawConnSeesDatabase verifies that a brand-new
// session finds the database the moment SetupSharedTestDB returns: it is
// listed by SHOW DATABASES (the probe dolt.New runs to choose between create
// and open) and USE <db> succeeds on it.
func TestSetupSharedTestDB_FreshRawConnSeesDatabase(t *testing.T) {
	skipIfNoServer(t)

	dbName, _ := setupSharedTestDB(t)

	if !databaseExists(t, testServerPort, dbName) {
		t.Fatalf("a fresh session's SHOW DATABASES does not list %q right after SetupSharedTestDB returned (be-s9d)", dbName)
	}

	fresh := rawTestConn(t, testServerPort)
	defer fresh.Close()
	fresh.SetMaxOpenConns(1)
	if _, err := fresh.Exec("USE `" + dbName + "`"); err != nil {
		t.Fatalf("USE %s on a fresh session right after SetupSharedTestDB returned: %v (be-s9d)", dbName, err)
	}
}

// TestSetupSharedTestDB_NewStoreOpens is the literal repro from be-s9d: open a
// store on the database SetupSharedTestDB just created. CreateIfMissing stays
// false, so a New that cannot see the database fails with "database not found"
// instead of quietly creating one.
func TestSetupSharedTestDB_NewStoreOpens(t *testing.T) {
	skipIfNoServer(t)

	dbName, _ := setupSharedTestDB(t)
	ctx, cancel := testContext(t)
	defer cancel()

	store, err := New(ctx, &Config{
		Path:         t.TempDir(),
		ServerHost:   "127.0.0.1",
		ServerPort:   testServerPort,
		Database:     dbName,
		MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatalf("New on %q right after SetupSharedTestDB: %v (be-s9d)", dbName, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// A write forces the store's connection to USE the database and execute.
	if err := store.SetConfig(ctx, "issue_prefix", "setupshared_test"); err != nil {
		t.Fatalf("SetConfig on a store opened right after SetupSharedTestDB: %v", err)
	}
}

// TestSetupSharedTestDB_FreshConnAfterSecondSetupCall verifies the idempotent
// path: a second SetupSharedTestDB on the same name (CREATE DATABASE IF NOT
// EXISTS, which Dolt may answer with error 1007) still leaves the database
// visible to a fresh session.
func TestSetupSharedTestDB_FreshConnAfterSecondSetupCall(t *testing.T) {
	skipIfNoServer(t)

	dbName, _ := setupSharedTestDB(t)

	second, err := testutil.SetupSharedTestDB(testServerPort, dbName)
	if err != nil {
		t.Fatalf("second SetupSharedTestDB on existing %q: %v", dbName, err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if !databaseExists(t, testServerPort, dbName) {
		t.Fatalf("a fresh session's SHOW DATABASES does not list %q after a second SetupSharedTestDB call", dbName)
	}
}

// TestInitSharedSchema_AfterFreshDatabase verifies the TestMain sequence
// end to end on a fresh database: SetupSharedTestDB, then initSharedSchema (New,
// migrations, DOLT_ADD + DOLT_COMMIT). The committed schema must be visible to
// a new session, or branches cut from main would start empty.
//
// initSharedSchema reads the package-level testSharedDB, so it is pointed at
// this test's database for the duration.
func TestInitSharedSchema_AfterFreshDatabase(t *testing.T) {
	skipIfNoServer(t)

	dbName, _ := setupSharedTestDB(t)

	prevDB := testSharedDB
	testSharedDB = dbName
	t.Cleanup(func() { testSharedDB = prevDB })

	if err := initSharedSchema(testServerPort); err != nil {
		t.Fatalf("initSharedSchema on %q right after SetupSharedTestDB: %v (be-s9d)", dbName, err)
	}

	fresh, err := sql.Open("mysql", doltutil.ServerDSN{
		Host:     "127.0.0.1",
		Port:     testServerPort,
		User:     "root",
		Database: dbName,
		Timeout:  5 * time.Second,
	}.String())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer fresh.Close()

	// issues is the canonical table the migrations create: its presence proves
	// the schema commit landed for sessions other than the one that made it.
	var n int
	if err := fresh.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM issues").Scan(&n); err != nil {
		t.Fatalf("issues table not visible to a fresh session after initSharedSchema: %v", err)
	}
}

// TestSetupSharedTestDB_MissingDatabaseIsMySQLError1049 pins the server answer
// the visibility wait in SetupSharedTestDB keys on. If a Dolt upgrade changed
// it, the wait would silently stop retrying and the be-s9d race would come back
// with every other test still green.
func TestSetupSharedTestDB_MissingDatabaseIsMySQLError1049(t *testing.T) {
	skipIfNoServer(t)

	conn := rawTestConn(t, testServerPort)
	defer conn.Close()
	conn.SetMaxOpenConns(1)

	_, err := conn.Exec("USE `" + uniqueTestDBName(t) + "`")
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1049 {
		t.Fatalf("USE on a missing database = %v, want a *mysql.MySQLError with number 1049", err)
	}
}

//go:build integration_pg

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/steveyegge/beads/internal/storage"
	_ "github.com/steveyegge/beads/internal/storage/postgres" // self-registers BackendPostgres
	"github.com/steveyegge/beads/internal/storage/postgres/dsn"
	"github.com/steveyegge/beads/internal/storage/postgres/testfixture"
)

// initPGFixture spins up a fresh PG database, applies the bd migration set
// via storage.Open, strips the password into metadata.json, exports
// BEADS_POSTGRES_PASSWORD, and returns the resolved repo path that the
// doctor checks consume.
func initPGFixture(t *testing.T) string {
	t.Helper()

	fullDSN := testfixture.ForTest(t)

	// Bring the schema up to date by opening the store once. RunPostgresHealthChecks
	// opens its own short-lived pool against the same DSN.
	ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000) // 30s in ns to avoid time import
	defer cancel()

	store, err := storage.Open(ctx, storage.BackendPostgres, storage.ConnectionConfig{DSN: fullDSN})
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}

	stripped, err := dsn.Strip(fullDSN)
	if err != nil {
		t.Fatalf("dsn.Strip: %v", err)
	}

	// Recover the password component from the full DSN so the doctor's
	// dsn.Compose(stripped, BEADS_POSTGRES_PASSWORD) call reconstructs the
	// usable runtime DSN.
	parsed, err := pgconn.ParseConfig(fullDSN)
	if err != nil {
		t.Fatalf("ParseConfig full DSN: %v", err)
	}
	t.Setenv("BEADS_POSTGRES_PASSWORD", parsed.Password)

	repo := t.TempDir()
	beadsDir := filepath.Join(repo, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}
	metadata := `{
  "backend": "postgres",
  "postgres_dsn": ` + strconvQuote(stripped) + `
}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	t.Cleanup(clearResolveBeadsDirCache)
	return repo
}

// strconvQuote is a local JSON-string quoter to avoid pulling in strconv for
// a one-line helper. The fixture DSN never contains tab/newline/etc, so this
// is sufficient.
func strconvQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func TestRunPostgresHealthChecks_RealServer(t *testing.T) {
	repo := initPGFixture(t)

	checks := RunPostgresHealthChecks(repo)
	if len(checks) != 4 {
		t.Fatalf("expected 4 checks, got %d", len(checks))
	}

	wantNames := []string{
		"Postgres Connection",
		"Postgres Schema Version",
		"Postgres Tables",
		"Postgres Activity",
	}
	for i, name := range wantNames {
		if checks[i].Name != name {
			t.Errorf("checks[%d].Name = %q, want %q", i, checks[i].Name, name)
		}
		if checks[i].Status != StatusOK {
			t.Errorf("checks[%d] (%s) = %s: %s — Detail: %s",
				i, name, checks[i].Status, checks[i].Message, checks[i].Detail)
		}
	}

	// Connection check should expose the host:port/db in Detail so an
	// operator can confirm bd is talking to the right server.
	if !strings.Contains(checks[0].Detail, "Storage: Postgres") {
		t.Errorf("connection Detail = %q, want 'Storage: Postgres ...'", checks[0].Detail)
	}

	// Schema version OK message should name the current version.
	if !strings.Contains(checks[1].Message, "Schema is at version") {
		t.Errorf("schema Message = %q, want 'Schema is at version N'", checks[1].Message)
	}

	// Empty DB → Activity reports the fresh-project shape.
	if !strings.Contains(checks[3].Message, "fresh project") {
		t.Errorf("activity Message = %q, want 'fresh project' on empty DB", checks[3].Message)
	}
}

func TestRunPostgresHealthChecks_AuthFailure(t *testing.T) {
	repo := initPGFixture(t)
	// Override the password env to a wrong value. The fixture admin user
	// requires the correct password (test container is configured that way).
	t.Setenv("BEADS_POSTGRES_PASSWORD", "definitely-not-the-real-password")

	checks := RunPostgresHealthChecks(repo)
	if checks[0].Status != StatusError {
		t.Fatalf("connection Status = %s, want StatusError on auth failure: %+v", checks[0].Status, checks[0])
	}
	if !strings.Contains(checks[0].Detail, "authentication") {
		t.Errorf("connection Detail = %q, want substring 'authentication'", checks[0].Detail)
	}
	// Downstream checks should be marked skipped.
	for i := 1; i < 4; i++ {
		if checks[i].Status != StatusError {
			t.Errorf("checks[%d] (%s) Status = %s, want StatusError when connection fails", i, checks[i].Name, checks[i].Status)
		}
	}
}

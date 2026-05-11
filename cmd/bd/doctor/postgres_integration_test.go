//go:build integration_pg

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/steveyegge/beads/internal/beads"
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
	if len(checks) != 5 {
		t.Fatalf("expected 5 checks, got %d", len(checks))
	}

	wantNames := []string{
		"Postgres Connection",
		"Postgres Schema Version",
		"Postgres Tables",
		"Postgres Activity",
		"Repo Fingerprint",
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

	// Repo Fingerprint on an initPGFixture-built repo: the fixture's t.TempDir
	// is not a git repo, so the check returns the N/A path. Specific seed /
	// verify / mismatch coverage lives in TestCheckPGRepoFingerprint_* below
	// where the test sets up a git-init'd repo.
	if !strings.Contains(checks[4].Message, "N/A (not a git repository)") {
		t.Errorf("fingerprint Message = %q, want 'N/A (not a git repository)' on non-git repo", checks[4].Message)
	}
}

// initPGFixtureGitRepo is initPGFixture + git init of the returned repo so
// beads.ComputeRepoIDForPath returns a real fingerprint instead of failing
// with "not a git repository". The two-step setup is intentional: the PG
// fixture is independent of git, and TestRunPostgresHealthChecks_RealServer
// validates the non-git N/A path. Fingerprint-specific tests need the git
// step on top.
func initPGFixtureGitRepo(t *testing.T) string {
	t.Helper()
	repo := initPGFixture(t)
	setupGitRepoInDir(t, repo)
	return repo
}

func TestCheckPGRepoFingerprint_SeedThenVerify(t *testing.T) {
	repo := initPGFixtureGitRepo(t)

	want, err := beads.ComputeRepoIDForPath(repo)
	if err != nil {
		t.Fatalf("ComputeRepoIDForPath: %v", err)
	}

	// First run: fresh DB (no marker yet) → expect Seeded.
	first := RunPostgresHealthChecks(repo)
	if len(first) != 5 {
		t.Fatalf("expected 5 checks, got %d", len(first))
	}
	fp := first[4]
	if fp.Name != "Repo Fingerprint" {
		t.Fatalf("checks[4].Name = %q, want \"Repo Fingerprint\"", fp.Name)
	}
	if fp.Status != StatusOK {
		t.Fatalf("first-run Status = %q, want StatusOK; Message=%q Detail=%q", fp.Status, fp.Message, fp.Detail)
	}
	if !strings.Contains(fp.Message, "Seeded") {
		t.Errorf("first-run Message = %q, want substring \"Seeded\"", fp.Message)
	}
	if !strings.Contains(fp.Message, want[:8]) {
		t.Errorf("first-run Message = %q, want substring %q (computed fingerprint)", fp.Message, want[:8])
	}

	// Second run: marker now present and matches → expect Verified.
	second := RunPostgresHealthChecks(repo)
	fp2 := second[4]
	if fp2.Status != StatusOK {
		t.Fatalf("second-run Status = %q, want StatusOK; Message=%q Detail=%q", fp2.Status, fp2.Message, fp2.Detail)
	}
	if !strings.Contains(fp2.Message, "Verified") {
		t.Errorf("second-run Message = %q, want substring \"Verified\"", fp2.Message)
	}
	if !strings.Contains(fp2.Message, want[:8]) {
		t.Errorf("second-run Message = %q, want substring %q (computed fingerprint)", fp2.Message, want[:8])
	}
}

func TestCheckPGRepoFingerprint_Mismatch(t *testing.T) {
	repo := initPGFixtureGitRepo(t)

	// Seed the marker via the first health-check run.
	if got := RunPostgresHealthChecks(repo)[4].Status; got != StatusOK {
		t.Fatalf("setup: first-run fingerprint Status = %q, want StatusOK", got)
	}

	// Replace the stored marker with a different value to simulate the
	// "operator pointed bd at a different cluster" hazard.
	beadsDir := filepath.Join(repo, ".beads")
	conn, err := openPGConn(beadsDir)
	if err != nil {
		t.Fatalf("openPGConn: %v", err)
	}
	ctx := context.Background()
	const otherFingerprint = "deadbeefdeadbeefdeadbeefdeadbeef"
	if _, err := conn.pool.Exec(ctx,
		`UPDATE metadata SET value = $1 WHERE key = 'repo_id'`,
		otherFingerprint,
	); err != nil {
		conn.Close()
		t.Fatalf("override stored repo_id: %v", err)
	}
	conn.Close()

	checks := RunPostgresHealthChecks(repo)
	fp := checks[4]
	if fp.Status != StatusError {
		t.Fatalf("Status = %q, want StatusError on mismatch; Message=%q Detail=%q", fp.Status, fp.Message, fp.Detail)
	}
	if !strings.Contains(fp.Message, "different repository") {
		t.Errorf("Message = %q, want substring \"different repository\"", fp.Message)
	}
	if !strings.Contains(fp.Detail, otherFingerprint[:8]) {
		t.Errorf("Detail = %q, want stored fingerprint prefix %q", fp.Detail, otherFingerprint[:8])
	}
	if !strings.Contains(fp.Detail, "different database than this workspace expects") {
		t.Errorf("Detail = %q, want clear wrong-DB phrasing", fp.Detail)
	}
	if fp.Fix == "" {
		t.Errorf("Fix is empty; want actionable remediation hint")
	}
}

func TestCheckPGRepoFingerprint_NotGitRepository(t *testing.T) {
	// initPGFixture sets up PG but NOT git, so ComputeRepoIDForPath returns
	// "not a git repository" and the check short-circuits to N/A.
	repo := initPGFixture(t)

	checks := RunPostgresHealthChecks(repo)
	fp := checks[4]
	if fp.Status != StatusOK {
		t.Fatalf("Status = %q, want StatusOK on non-git repo; Message=%q Detail=%q", fp.Status, fp.Message, fp.Detail)
	}
	if !strings.Contains(fp.Message, "N/A (not a git repository)") {
		t.Errorf("Message = %q, want \"N/A (not a git repository)\"", fp.Message)
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
	for i := 1; i < 5; i++ {
		if checks[i].Status != StatusError {
			t.Errorf("checks[%d] (%s) Status = %s, want StatusError when connection fails", i, checks[i].Name, checks[i].Status)
		}
	}
}

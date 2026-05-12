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
	if len(checks) != 7 {
		t.Fatalf("expected 7 checks, got %d", len(checks))
	}

	wantNames := []string{
		"Postgres Connection",
		"Postgres Schema Version",
		"Postgres Tables",
		"Postgres Activity",
		"Repo Fingerprint",
		"Referential Integrity",
		"Dependency Cycles",
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

	// Referential Integrity on a fresh DB has nothing to find.
	if !strings.Contains(checks[5].Message, "All cross-table references resolve") {
		t.Errorf("integrity Message = %q, want 'All cross-table references resolve' on clean DB", checks[5].Message)
	}

	// Dependency Cycles on a fresh DB has no edges to walk.
	if !strings.Contains(checks[6].Message, "No circular dependencies detected") {
		t.Errorf("cycles Message = %q, want 'No circular dependencies detected' on clean DB", checks[6].Message)
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
	if len(first) != 7 {
		t.Fatalf("expected 7 checks, got %d", len(first))
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
	for i := 1; i < 7; i++ {
		if checks[i].Status != StatusError {
			t.Errorf("checks[%d] (%s) Status = %s, want StatusError when connection fails", i, checks[i].Name, checks[i].Status)
		}
	}
}

func TestCheckPGReferentialIntegrity_Clean(t *testing.T) {
	repo := initPGFixture(t)

	checks := RunPostgresHealthChecks(repo)
	ri := checks[5]
	if ri.Name != "Referential Integrity" {
		t.Fatalf("checks[5].Name = %q, want \"Referential Integrity\"", ri.Name)
	}
	if ri.Status != StatusOK {
		t.Fatalf("Status = %q, want StatusOK on fresh DB; Message=%q Detail=%q", ri.Status, ri.Message, ri.Detail)
	}
	if !strings.Contains(ri.Message, "All cross-table references resolve") {
		t.Errorf("Message = %q, want clean-DB phrasing", ri.Message)
	}
}

func TestCheckPGReferentialIntegrity_OrphanInDependencies(t *testing.T) {
	repo := initPGFixture(t)
	beadsDir := filepath.Join(repo, ".beads")

	conn, err := openPGConn(beadsDir)
	if err != nil {
		t.Fatalf("openPGConn: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	// Seed a real parent issue so the dependencies.issue_id FK is satisfied.
	// title/description/design/acceptance_criteria/notes are all NOT NULL.
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO issues (id, title, description, design, acceptance_criteria, notes,
		                     status, priority, issue_type, owner)
		 VALUES ('bd-aaaaaa', 'parent', '', '', '', '', 'open', 2, 'task', 'tester')`,
	); err != nil {
		t.Fatalf("seed parent issue: %v", err)
	}

	// Insert a dependency whose depends_on_id has no matching issue or wisp.
	// dependencies.depends_on_id has NO FK in the PG schema — that is exactly
	// the diagnostic gap this check exists to close.
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO dependencies (issue_id, depends_on_id, type)
		 VALUES ('bd-aaaaaa', 'bd-zzzzzz-orphan', 'blocks')`,
	); err != nil {
		t.Fatalf("insert orphan dep: %v", err)
	}

	checks := RunPostgresHealthChecks(repo)
	ri := checks[5]
	if ri.Status != StatusError {
		t.Fatalf("Status = %q, want StatusError when orphan present; Message=%q Detail=%q", ri.Status, ri.Message, ri.Detail)
	}
	if !strings.Contains(ri.Detail, "dependencies.depends_on_id") {
		t.Errorf("Detail = %q, want it to name dependencies.depends_on_id", ri.Detail)
	}
	if !strings.Contains(ri.Detail, "bd-zzzzzz-orphan") {
		t.Errorf("Detail = %q, want it to surface the orphan ID", ri.Detail)
	}
	if !strings.Contains(ri.Message, "orphan rows in 1 location") {
		t.Errorf("Message = %q, want single-location summary", ri.Message)
	}
	if ri.Fix == "" {
		t.Errorf("Fix is empty; want a remediation hint")
	}
}

func TestCheckPGReferentialIntegrity_OrphanInMultipleWispTables(t *testing.T) {
	repo := initPGFixture(t)
	beadsDir := filepath.Join(repo, ".beads")

	conn, err := openPGConn(beadsDir)
	if err != nil {
		t.Fatalf("openPGConn: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	// wisp_* tables ship with ZERO FKs (be-4i7ax3 §4 D-5). Any insert lands
	// without parent rows in `wisps` — that is the hazard.
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO wisp_labels (issue_id, label) VALUES ('orphan-wisp-1', 'stale')`,
	); err != nil {
		t.Fatalf("insert orphan wisp_label: %v", err)
	}
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO wisp_comments (issue_id, author, text) VALUES ('orphan-wisp-2', 'tester', 'comment')`,
	); err != nil {
		t.Fatalf("insert orphan wisp_comment: %v", err)
	}
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO wisp_events (issue_id, event_type, actor) VALUES ('orphan-wisp-3', 'created', 'tester')`,
	); err != nil {
		t.Fatalf("insert orphan wisp_event: %v", err)
	}

	checks := RunPostgresHealthChecks(repo)
	ri := checks[5]
	if ri.Status != StatusError {
		t.Fatalf("Status = %q, want StatusError when orphans present; Message=%q Detail=%q", ri.Status, ri.Message, ri.Detail)
	}
	if !strings.Contains(ri.Message, "orphan rows in 3 location") {
		t.Errorf("Message = %q, want 3-location summary", ri.Message)
	}
	for _, label := range []string{"wisp_labels.issue_id", "wisp_comments.issue_id", "wisp_events.issue_id"} {
		if !strings.Contains(ri.Detail, label) {
			t.Errorf("Detail = %q, want it to name %s", ri.Detail, label)
		}
	}
}

// seedCycleIssue inserts a minimal issues row so the dependencies FK on
// issue_id is satisfied. Cycle-detection tests need real parent rows for
// every node in the graph — the depends_on_id column has no FK, but
// issue_id does (see internal/storage/postgres/migrations/0001_initial.up.sql).
func seedCycleIssue(t *testing.T, conn *pgConn, id string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO issues (id, title, description, design, acceptance_criteria, notes,
		                     status, priority, issue_type, owner)
		 VALUES ($1, $2, '', '', '', '', 'open', 2, 'task', 'tester')`,
		id, id+" node",
	); err != nil {
		t.Fatalf("seed cycle node %s: %v", id, err)
	}
}

func TestCheckPGDependencyCycles_Clean(t *testing.T) {
	repo := initPGFixture(t)

	checks := RunPostgresHealthChecks(repo)
	dc := checks[6]
	if dc.Name != "Dependency Cycles" {
		t.Fatalf("checks[6].Name = %q, want \"Dependency Cycles\"", dc.Name)
	}
	if dc.Status != StatusOK {
		t.Fatalf("Status = %q, want StatusOK on fresh DB; Message=%q Detail=%q", dc.Status, dc.Message, dc.Detail)
	}
	if !strings.Contains(dc.Message, "No circular dependencies detected") {
		t.Errorf("Message = %q, want clean-DB phrasing", dc.Message)
	}
}

func TestCheckPGDependencyCycles_TwoNodeCycle(t *testing.T) {
	repo := initPGFixture(t)
	beadsDir := filepath.Join(repo, ".beads")

	conn, err := openPGConn(beadsDir)
	if err != nil {
		t.Fatalf("openPGConn: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	seedCycleIssue(t, conn, "bd-cyclea")
	seedCycleIssue(t, conn, "bd-cycleb")

	// bd-cyclea → bd-cycleb → bd-cyclea
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES
		   ('bd-cyclea', 'bd-cycleb', 'blocks'),
		   ('bd-cycleb', 'bd-cyclea', 'blocks')`,
	); err != nil {
		t.Fatalf("insert 2-node cycle: %v", err)
	}

	checks := RunPostgresHealthChecks(repo)
	dc := checks[6]
	if dc.Status != StatusError {
		t.Fatalf("Status = %q, want StatusError when cycle present; Message=%q Detail=%q", dc.Status, dc.Message, dc.Detail)
	}
	if !strings.Contains(dc.Message, "circular dependency cycle") {
		t.Errorf("Message = %q, want substring \"circular dependency cycle\"", dc.Message)
	}
	// Sample path must surface both nodes and close back on the start.
	if !strings.Contains(dc.Detail, "bd-cyclea") || !strings.Contains(dc.Detail, "bd-cycleb") {
		t.Errorf("Detail = %q, want it to name both cycle members", dc.Detail)
	}
	if !strings.Contains(dc.Detail, "→") {
		t.Errorf("Detail = %q, want it to render the cycle path with arrows", dc.Detail)
	}
	if dc.Fix == "" {
		t.Errorf("Fix is empty; want actionable remediation hint")
	}
}

func TestCheckPGDependencyCycles_ThreeNodeCycle(t *testing.T) {
	repo := initPGFixture(t)
	beadsDir := filepath.Join(repo, ".beads")

	conn, err := openPGConn(beadsDir)
	if err != nil {
		t.Fatalf("openPGConn: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	seedCycleIssue(t, conn, "bd-tria")
	seedCycleIssue(t, conn, "bd-trib")
	seedCycleIssue(t, conn, "bd-tric")

	// bd-tria → bd-trib → bd-tric → bd-tria
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES
		   ('bd-tria', 'bd-trib', 'blocks'),
		   ('bd-trib', 'bd-tric', 'blocks'),
		   ('bd-tric', 'bd-tria', 'blocks')`,
	); err != nil {
		t.Fatalf("insert 3-node cycle: %v", err)
	}

	checks := RunPostgresHealthChecks(repo)
	dc := checks[6]
	if dc.Status != StatusError {
		t.Fatalf("Status = %q, want StatusError when cycle present; Message=%q Detail=%q", dc.Status, dc.Message, dc.Detail)
	}
	for _, id := range []string{"bd-tria", "bd-trib", "bd-tric"} {
		if !strings.Contains(dc.Detail, id) {
			t.Errorf("Detail = %q, want it to name cycle member %q", dc.Detail, id)
		}
	}
}

func TestCheckPGDependencyCycles_NoCycleOnLinearChain(t *testing.T) {
	repo := initPGFixture(t)
	beadsDir := filepath.Join(repo, ".beads")

	conn, err := openPGConn(beadsDir)
	if err != nil {
		t.Fatalf("openPGConn: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	// A → B → C with no back-edge: classic DAG, must NOT be flagged.
	seedCycleIssue(t, conn, "bd-lina")
	seedCycleIssue(t, conn, "bd-linb")
	seedCycleIssue(t, conn, "bd-linc")
	if _, err := conn.pool.Exec(ctx,
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES
		   ('bd-lina', 'bd-linb', 'blocks'),
		   ('bd-linb', 'bd-linc', 'blocks')`,
	); err != nil {
		t.Fatalf("insert linear chain: %v", err)
	}

	checks := RunPostgresHealthChecks(repo)
	dc := checks[6]
	if dc.Status != StatusOK {
		t.Fatalf("Status = %q, want StatusOK on DAG; Message=%q Detail=%q", dc.Status, dc.Message, dc.Detail)
	}
}

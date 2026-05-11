package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/postgres/dsn"
)

// pgConnectTimeout bounds how long doctor will wait for a postgres connection
// to come up. Doctor must remain responsive even when the configured host is
// unreachable — a long default connect timeout (libpq's is forever) would
// otherwise hang the whole diagnostic run.
const pgConnectTimeout = 10 * time.Second

// pgCheckRequiredTables enumerates the tables that must exist after the
// initial postgres migration set has been applied. Missing entries indicate
// schema drift (e.g. partial migration, manual drops, wrong database).
var pgCheckRequiredTables = []string{
	"issues",
	"dependencies",
	"labels",
	"comments",
	"events",
	"wisps",
	"bd_schema_migrations",
	"config",
}

// IsPostgresBackend reports whether the beads dir is configured for the
// postgres backend. Returns false when config is missing or unreadable —
// the dolt path is the safe default for malformed configs.
func IsPostgresBackend(beadsDir string) bool {
	cfg, err := configfile.Load(beadsDir)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.GetBackend() == configfile.BackendPostgres
}

// pgConn holds an open postgres pool for a doctor run. The pool is sized
// minimally (one connection) — doctor checks are sequential and a wider pool
// would just hold extra DB sessions open.
type pgConn struct {
	pool *pgxpool.Pool
	cfg  *configfile.Config
	host string // non-credential identifier for messages
}

// Close releases the underlying pool. Safe to call multiple times.
func (c *pgConn) Close() {
	if c == nil || c.pool == nil {
		return
	}
	c.pool.Close()
	c.pool = nil
}

// openPGConn loads the configured DSN, composes it with the runtime password
// from BEADS_POSTGRES_PASSWORD (if set), and opens a pool with a bounded
// connect timeout. The first connection's failure (auth, host unreachable,
// connect timeout) is what bubbles up — pgxpool.New does not validate eagerly
// without a ping, so we ping after construction.
func openPGConn(beadsDir string) (*pgConn, error) {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("load metadata.json: %w", err)
	}
	if cfg == nil {
		return nil, errors.New("no beads configuration in this directory")
	}
	if cfg.PostgresDSN == "" {
		return nil, errors.New("metadata.json has backend=postgres but postgres_dsn is empty")
	}

	composed := dsn.Compose(cfg.PostgresDSN, os.Getenv("BEADS_POSTGRES_PASSWORD"))

	pgxCfg, err := pgxpool.ParseConfig(composed)
	if err != nil {
		// pgconn.ParseConfig echoes the raw DSN on error; keep our message
		// generic so a credential never lands in stderr (mirror dsn.Strip).
		return nil, errors.New("could not parse postgres_dsn (run `bd doctor` after fixing metadata.json)")
	}

	// One connection is enough for sequential doctor checks. A wider pool
	// wastes server slots and slows down the close path.
	pgxCfg.MaxConns = 1
	pgxCfg.MinConns = 0
	if pgxCfg.ConnConfig != nil {
		pgxCfg.ConnConfig.ConnectTimeout = pgConnectTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), pgConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, classifyPGError(err)
	}

	// pgxpool.NewWithConfig does not validate the connection — force the
	// first connect so auth/timeout failures surface here instead of later.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, classifyPGError(err)
	}

	return &pgConn{pool: pool, cfg: cfg, host: pgHostInfo(pgxCfg)}, nil
}

// pgHostInfo extracts a credential-free identifier for doctor output.
func pgHostInfo(pgxCfg *pgxpool.Config) string {
	if pgxCfg == nil || pgxCfg.ConnConfig == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d/%s",
		pgxCfg.ConnConfig.Host,
		pgxCfg.ConnConfig.Port,
		pgxCfg.ConnConfig.Database)
}

// classifyPGError categorizes connection failures into the operator-facing
// taxonomy from be-flo2kl: auth, host unreachable, connect timeout, server
// version, and generic. The returned error wraps a one-line, credential-free
// message; the doctor check layer uses it directly as the check Detail.
func classifyPGError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	// pgconn surfaces a structured *pgconn.PgError for server-rejected
	// connections (e.g. password auth failed). Use that when available.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "28P01", "28000":
			return errors.New("authentication failed: check BEADS_POSTGRES_PASSWORD or .pgpass")
		case "3D000":
			return errors.New("database does not exist on the configured host")
		case "57P03":
			return errors.New("server is starting up; retry shortly")
		}
		return fmt.Errorf("server error %s: %s", pgErr.Code, pgErr.Message)
	}

	switch {
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "i/o timeout"):
		return fmt.Errorf("connect timeout (> %s); is the host reachable?", pgConnectTimeout)
	case strings.Contains(msg, "connection refused"):
		return errors.New("connection refused: postgres is not listening on the configured host:port")
	case strings.Contains(msg, "no such host"),
		strings.Contains(msg, "no route to host"):
		return errors.New("host unreachable: DNS does not resolve or no route to the configured host")
	case strings.Contains(msg, "password authentication failed"):
		return errors.New("authentication failed: check BEADS_POSTGRES_PASSWORD or .pgpass")
	case strings.Contains(strings.ToLower(msg), "ssl"),
		strings.Contains(strings.ToLower(msg), "tls"):
		return errors.New("TLS handshake failed: check sslmode and certificate configuration")
	}
	return err
}

// RunPostgresHealthChecks runs the postgres-aware doctor checks for a beads
// directory configured with backend=postgres. The slice mirrors the shape
// of RunDoltHealthChecks: one check per dimension, with N/A entries when the
// backend is not postgres so JSON consumers see a stable check layout.
func RunPostgresHealthChecks(path string) []DoctorCheck {
	beadsDir := ResolveBeadsDirForRepo(path)

	if !IsPostgresBackend(beadsDir) {
		na := "N/A (not using Postgres backend)"
		return []DoctorCheck{
			{Name: "Postgres Connection", Status: StatusOK, Message: na, Category: CategoryCore},
			{Name: "Postgres Schema Version", Status: StatusOK, Message: na, Category: CategoryCore},
			{Name: "Postgres Tables", Status: StatusOK, Message: na, Category: CategoryCore},
			{Name: "Postgres Activity", Status: StatusOK, Message: na, Category: CategoryData},
			{Name: "Repo Fingerprint", Status: StatusOK, Message: na, Category: CategoryCore},
		}
	}

	conn, err := openPGConn(beadsDir)
	if err != nil {
		failMsg := err.Error()
		fix := "Verify BEADS_POSTGRES_PASSWORD, postgres_dsn in metadata.json, and that the server is reachable"
		return []DoctorCheck{
			{
				Name:     "Postgres Connection",
				Status:   StatusError,
				Message:  "Failed to connect to Postgres",
				Detail:   failMsg,
				Fix:      fix,
				Category: CategoryCore,
			},
			{Name: "Postgres Schema Version", Status: StatusError, Message: "Skipped (no connection)", Detail: failMsg, Category: CategoryCore},
			{Name: "Postgres Tables", Status: StatusError, Message: "Skipped (no connection)", Detail: failMsg, Category: CategoryCore},
			{Name: "Postgres Activity", Status: StatusError, Message: "Skipped (no connection)", Detail: failMsg, Category: CategoryData},
			{Name: "Repo Fingerprint", Status: StatusError, Message: "Skipped (no connection)", Detail: failMsg, Category: CategoryCore},
		}
	}
	defer conn.Close()

	return []DoctorCheck{
		checkPGConnection(conn),
		checkPGSchemaVersion(conn),
		checkPGTables(conn),
		checkPGRecentActivity(conn),
		checkPGRepoFingerprint(conn, path),
	}
}

func checkPGConnection(conn *pgConn) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.pool.Ping(ctx); err != nil {
		return DoctorCheck{
			Name:     "Postgres Connection",
			Status:   StatusError,
			Message:  "Ping failed after pool open",
			Detail:   classifyPGError(err).Error(),
			Category: CategoryCore,
		}
	}

	return DoctorCheck{
		Name:     "Postgres Connection",
		Status:   StatusOK,
		Message:  "Connected successfully",
		Detail:   "Storage: Postgres (" + conn.host + ")",
		Category: CategoryCore,
	}
}

// pgLatestEmbeddedVersion mirrors postgres.latestEmbeddedVersion. Kept in
// sync manually because the embedded migration list is intentionally private
// to the postgres package — doctor only needs the count, not the bodies.
//
// Update this when adding new entries to embeddedMigrations in
// internal/storage/postgres/schema.go.
const pgLatestEmbeddedVersion = 3

func checkPGSchemaVersion(conn *pgConn) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var current int
	err := conn.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM bd_schema_migrations`,
	).Scan(&current)
	if err != nil {
		// Most likely cause: bd_schema_migrations was never created — i.e.
		// `bd init --backend=postgres` was not run against this database, or
		// the schema was dropped manually. We surface that explicitly rather
		// than echoing the pgx "relation does not exist" sentinel.
		return DoctorCheck{
			Name:     "Postgres Schema Version",
			Status:   StatusError,
			Message:  "Schema not initialized (bd_schema_migrations missing)",
			Detail:   err.Error(),
			Fix:      "Run `bd init --backend=postgres --dsn=...` to apply the initial migration set",
			Category: CategoryCore,
		}
	}

	switch {
	case current == 0:
		return DoctorCheck{
			Name:     "Postgres Schema Version",
			Status:   StatusError,
			Message:  "No migrations applied",
			Detail:   "bd_schema_migrations exists but is empty",
			Fix:      "Run `bd init --backend=postgres --dsn=...` to apply migrations",
			Category: CategoryCore,
		}
	case current < pgLatestEmbeddedVersion:
		return DoctorCheck{
			Name:     "Postgres Schema Version",
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Schema is behind bd (applied=%d, expected=%d)", current, pgLatestEmbeddedVersion),
			Detail:   "Pending migrations will run on the next bd command",
			Fix:      "Run any bd command (e.g. `bd ready`) to trigger the migration",
			Category: CategoryCore,
		}
	case current > pgLatestEmbeddedVersion:
		return DoctorCheck{
			Name:     "Postgres Schema Version",
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Schema is ahead of bd (applied=%d, this bd knows=%d)", current, pgLatestEmbeddedVersion),
			Detail:   "Another bd version applied newer migrations — this binary may not understand all rows",
			Fix:      "Upgrade bd to a build that includes the newer schema",
			Category: CategoryCore,
		}
	}

	return DoctorCheck{
		Name:     "Postgres Schema Version",
		Status:   StatusOK,
		Message:  fmt.Sprintf("Schema is at version %d", current),
		Category: CategoryCore,
	}
}

func checkPGTables(conn *pgConn) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := conn.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = current_schema()
		  AND table_type = 'BASE TABLE'
	`)
	if err != nil {
		return DoctorCheck{
			Name:     "Postgres Tables",
			Status:   StatusError,
			Message:  "Could not enumerate tables",
			Detail:   err.Error(),
			Category: CategoryCore,
		}
	}
	defer rows.Close()

	present := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return DoctorCheck{
				Name:     "Postgres Tables",
				Status:   StatusError,
				Message:  "Failed to read table list",
				Detail:   err.Error(),
				Category: CategoryCore,
			}
		}
		present[name] = true
	}
	if err := rows.Err(); err != nil {
		return DoctorCheck{
			Name:     "Postgres Tables",
			Status:   StatusError,
			Message:  "Failed to read table list",
			Detail:   err.Error(),
			Category: CategoryCore,
		}
	}

	var missing []string
	for _, t := range pgCheckRequiredTables {
		if !present[t] {
			missing = append(missing, t)
		}
	}

	if len(missing) > 0 {
		return DoctorCheck{
			Name:     "Postgres Tables",
			Status:   StatusError,
			Message:  fmt.Sprintf("Missing tables: %s", strings.Join(missing, ", ")),
			Fix:      "Run `bd init --backend=postgres --dsn=...` to recreate the schema, or restore from a backup",
			Category: CategoryCore,
		}
	}

	return DoctorCheck{
		Name:     "Postgres Tables",
		Status:   StatusOK,
		Message:  fmt.Sprintf("All %d required tables present", len(pgCheckRequiredTables)),
		Category: CategoryCore,
	}
}

func checkPGRecentActivity(conn *pgConn) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var lastUpdate *time.Time
	err := conn.pool.QueryRow(ctx, `SELECT MAX(updated_at) FROM issues`).Scan(&lastUpdate)
	if err != nil {
		return DoctorCheck{
			Name:     "Postgres Activity",
			Status:   StatusWarning,
			Message:  "Could not query issues.updated_at",
			Detail:   err.Error(),
			Category: CategoryData,
		}
	}

	if lastUpdate == nil {
		return DoctorCheck{
			Name:     "Postgres Activity",
			Status:   StatusOK,
			Message:  "No issues yet (fresh project)",
			Category: CategoryData,
		}
	}

	age := time.Since(*lastUpdate)
	human := lastUpdate.UTC().Format(time.RFC3339)
	return DoctorCheck{
		Name:     "Postgres Activity",
		Status:   StatusOK,
		Message:  fmt.Sprintf("Last write %s ago", roundDuration(age)),
		Detail:   "Most recent issues.updated_at: " + human,
		Category: CategoryData,
	}
}

// checkPGRepoFingerprint validates that the configured Postgres database
// belongs to this repository. The hazard mirrors the Dolt-side check
// (CheckRepoFingerprintWithStore in integrity.go): an operator switches the
// DSN to a different cluster, metadata.json keeps pointing at the prior
// fingerprint, and bd silently writes into the wrong database.
//
// The marker lives in the metadata table under key "repo_id" — the same key
// and value format the Dolt backend uses. That parity is intentional so
// (a) the bd doctor --fix path (cmd/bd/doctor/fix/metadata.go) repairs both
// backends through one SetMetadata call, and (b) operators reading the row
// directly see the same shape on both backends.
//
// First-run semantics: when no marker is present (fresh DB), the current
// repo_id is seeded into the metadata table and the check returns StatusOK.
// The seed uses INSERT … ON CONFLICT DO NOTHING so it is idempotent even
// under concurrent first-run races between two bd processes.
func checkPGRepoFingerprint(conn *pgConn, path string) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	currentRepoID, err := beads.ComputeRepoIDForPath(path)
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return DoctorCheck{
				Name:     "Repo Fingerprint",
				Status:   StatusOK,
				Message:  "N/A (not a git repository)",
				Category: CategoryCore,
			}
		}
		return DoctorCheck{
			Name:     "Repo Fingerprint",
			Status:   StatusWarning,
			Message:  "Unable to compute current repo ID",
			Detail:   err.Error(),
			Category: CategoryCore,
		}
	}

	var storedRepoID string
	err = conn.pool.QueryRow(ctx,
		`SELECT value FROM metadata WHERE key = 'repo_id'`,
	).Scan(&storedRepoID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DoctorCheck{
			Name:     "Repo Fingerprint",
			Status:   StatusWarning,
			Message:  "Unable to read repo fingerprint",
			Detail:   err.Error(),
			Category: CategoryCore,
		}
	}

	if errors.Is(err, pgx.ErrNoRows) || storedRepoID == "" {
		if _, werr := conn.pool.Exec(ctx,
			`INSERT INTO metadata (key, value) VALUES ('repo_id', $1)
			 ON CONFLICT (key) DO NOTHING`,
			currentRepoID,
		); werr != nil {
			return DoctorCheck{
				Name:     "Repo Fingerprint",
				Status:   StatusWarning,
				Message:  "Failed to seed repo fingerprint",
				Detail:   werr.Error(),
				Category: CategoryCore,
			}
		}
		return DoctorCheck{
			Name:     "Repo Fingerprint",
			Status:   StatusOK,
			Message:  fmt.Sprintf("Seeded (%s)", truncateID(currentRepoID)),
			Category: CategoryCore,
		}
	}

	if storedRepoID != currentRepoID {
		return DoctorCheck{
			Name:    "Repo Fingerprint",
			Status:  StatusError,
			Message: "Database belongs to different repository",
			Detail: fmt.Sprintf(
				"stored: %s, current: %s — you connected to a different database than this workspace expects",
				truncateID(storedRepoID),
				truncateID(currentRepoID),
			),
			Fix:      "Verify postgres_dsn in metadata.json points at the intended database; if the git remote URL changed, run 'bd migrate --update-repo-id'",
			Category: CategoryCore,
		}
	}

	return DoctorCheck{
		Name:     "Repo Fingerprint",
		Status:   StatusOK,
		Message:  fmt.Sprintf("Verified (%s)", truncateID(currentRepoID)),
		Category: CategoryCore,
	}
}

// roundDuration formats an age in a human-readable form bounded to the
// dominant unit. Doctor prints these inline so terse output is best.
func roundDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

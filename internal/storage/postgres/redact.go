package postgres

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

// dsnURLRegex matches Postgres URL DSNs anywhere in a string.
//
// We anchor on the protocol prefix (postgres:// or postgresql://) and consume
// non-whitespace characters; this is intentionally generous so that pgx
// internals that embed a partially-formed DSN in error text are still scrubbed.
var dsnURLRegex = regexp.MustCompile(`postgres(?:ql)?://[^\s]*`)

// passwordParamRegex matches `password=...` and `pgpassword=...` in libpq-style
// key/value DSNs and in URL query strings. Captures the key so the substitution
// can preserve it.
var passwordParamRegex = regexp.MustCompile(`(?i)(\bpassword|\bpgpassword)=[^\s&]*`)

// redactDSN scrubs Postgres DSN-looking substrings from the input. Used as a
// backstop on error wrapping; bd code does not log raw DSNs, but pgx internals
// can include them in error messages.
func redactDSN(s string) string {
	s = dsnURLRegex.ReplaceAllString(s, "[REDACTED_DSN]")
	s = passwordParamRegex.ReplaceAllString(s, "${1}=[REDACTED]")
	return s
}

// wrapErr wraps an error with an operation prefix and DSN redaction. nil input
// returns nil. SQLSTATE 28P01/28000 pgconn errors return a typed
// *ErrAuthFailure so callers can detect auth failure via errors.As and the
// user-visible message names BEADS_POSTGRES_PASSWORD as the env-var indirection.
func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "28P01" || pgErr.Code == "28000") {
		return &ErrAuthFailure{
			PGCode:   pgErr.Code,
			Op:       op,
			EnvUnset: os.Getenv("BEADS_POSTGRES_PASSWORD") == "",
			Cause:    errors.New(redactDSN(err.Error())),
		}
	}
	return fmt.Errorf("postgres: %s: %s", op, redactDSN(err.Error()))
}

// errStringWithCause builds a redacted error from a literal message and a cause.
func errStringWithCause(msg string, cause error) error {
	if cause == nil {
		return errors.New(redactDSN(msg))
	}
	return fmt.Errorf("%s: %s", redactDSN(msg), redactDSN(cause.Error()))
}

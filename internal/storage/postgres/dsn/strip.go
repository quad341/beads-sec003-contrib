// Package dsn provides Postgres DSN parsing and credential-stripping helpers
// for bd's persistence layer. The stripped form is what lands in
// metadata.json; the password is recovered at runtime from the
// BEADS_POSTGRES_PASSWORD env var (or PG-native sources like .pgpass).
package dsn

import (
	"errors"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Strip parses the DSN, removes the password component, and re-marshals it
// for persistence. Both URI form ("postgres://user:pw@host/db") and keyword
// form ("password=pw user=...") are accepted — pgconn.ParseConfig validates
// both.
//
// On parse error the wrapped pgconn error is intentionally dropped because
// pgconn's Error() text echoes the raw input — preserving it would surface
// credential-bearing strings in stderr / logs.
//
// For URI inputs the password is removed via textual edit on the parsed
// net/url, so all query-string parameters (sslmode included) round-trip
// verbatim. pgconn collapses sslmode into *tls.Config and drops it from
// RuntimeParams, so going through ConfigToConnString alone silently
// rewrites sslmode={allow,prefer,verify-ca,verify-full} → {disable,require}
// (be-g4x0eq). Keyword inputs go through pgconn but recover sslmode from
// the raw input first.
func Strip(rawDSN string) (string, error) {
	if rawDSN == "" {
		return "", errors.New("postgres: empty connection string")
	}
	if _, err := pgconn.ParseConfig(rawDSN); err != nil {
		return "", errors.New("postgres: invalid connection string")
	}
	if isURIForm(rawDSN) {
		return stripURIPassword(rawDSN)
	}
	return stripKeywordPassword(rawDSN)
}

// Compose returns a complete DSN by injecting password into a stripped DSN.
// If password is empty, returns the stripped form unchanged so PG can fall
// through to .pgpass / IAM / peer auth.
//
// URI inputs are edited textually so sslmode and other params round-trip
// verbatim; keyword inputs recover sslmode from the raw text before going
// through ConfigToConnString.
func Compose(strippedDSN, password string) string {
	if password == "" {
		return strippedDSN
	}
	if _, err := pgconn.ParseConfig(strippedDSN); err != nil {
		// Pass through; the postgres factory's own ParseConfig will surface
		// a clean error with redaction.
		return strippedDSN
	}
	if isURIForm(strippedDSN) {
		return injectURIPassword(strippedDSN, password)
	}
	return injectKeywordPassword(strippedDSN, password)
}

// isURIForm reports whether the DSN is libpq URI form (vs keyword form).
func isURIForm(s string) bool {
	return strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://")
}

// stripURIPassword removes the password from a URI-form DSN by editing the
// parsed net/url, leaving everything else (host, port, db, query string)
// verbatim.
func stripURIPassword(rawDSN string) (string, error) {
	u, err := url.Parse(rawDSN)
	if err != nil {
		// pgconn already validated so this should not happen, but be defensive.
		return "", errors.New("postgres: invalid connection string")
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.String(), nil
}

// injectURIPassword writes password into the userinfo of a URI-form DSN,
// preserving everything else verbatim.
func injectURIPassword(rawDSN, password string) string {
	u, err := url.Parse(rawDSN)
	if err != nil {
		return rawDSN
	}
	user := ""
	if u.User != nil {
		user = u.User.Username()
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}

// stripKeywordPassword converts a keyword-form DSN to URI form with the
// password removed. The conversion goes through pgconn for the well-known
// fields, but sslmode is recovered from the raw input because pgconn
// collapses it into *tls.Config and drops it from RuntimeParams.
func stripKeywordPassword(rawDSN string) (string, error) {
	cfg, err := pgconn.ParseConfig(rawDSN)
	if err != nil {
		return "", errors.New("postgres: invalid connection string")
	}
	cfg.Password = ""
	preserveKeywordSSLMode(cfg, rawDSN)
	return ConfigToConnString(cfg), nil
}

// injectKeywordPassword converts a keyword-form DSN to URI form with the
// supplied password. Defensive only — Strip emits URI form, so post-Strip
// callers will not hit this path. Kept symmetric with stripKeywordPassword
// in case an operator hand-edits metadata.json into keyword form.
func injectKeywordPassword(rawDSN, password string) string {
	cfg, err := pgconn.ParseConfig(rawDSN)
	if err != nil {
		return rawDSN
	}
	cfg.Password = password
	preserveKeywordSSLMode(cfg, rawDSN)
	return ConfigToConnString(cfg)
}

// preserveKeywordSSLMode pulls sslmode from raw keyword-form text and stuffs
// it back into cfg.RuntimeParams so ConfigToConnString emits the operator's
// original value rather than the lossy nil/non-nil-TLS approximation.
func preserveKeywordSSLMode(cfg *pgconn.Config, raw string) {
	mode := extractKeywordSSLMode(raw)
	if mode == "" {
		return
	}
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	cfg.RuntimeParams["sslmode"] = mode
}

// extractKeywordSSLMode scans a keyword-form DSN for the sslmode token.
// Handles bare and single-quoted values; whitespace-delimited per libpq.
func extractKeywordSSLMode(raw string) string {
	for _, tok := range strings.Fields(raw) {
		const key = "sslmode="
		if !strings.HasPrefix(tok, key) {
			continue
		}
		v := strings.TrimPrefix(tok, key)
		v = strings.Trim(v, "'")
		return v
	}
	return ""
}

// ConfigToConnString re-marshals a *pgconn.Config back into a URI-form DSN.
// pgx/v5 does not export an inverse for ParseConfig; this helper composes
// the URI from the well-known fields. Round-trip stable for inputs that
// don't carry exotic libpq extensions (covered by Strip's test suite).
//
// The output preserves whatever password is on cfg.Password — Strip clears
// it before calling, Compose sets it before calling.
//
// NOTE: pgconn.ParseConfig collapses sslmode into TLSConfig and drops it
// from RuntimeParams, so this function alone cannot recover the operator's
// original sslmode for soft values (allow, prefer, verify-ca, verify-full).
// Strip/Compose handle that by either taking the textual URI path or
// pre-populating cfg.RuntimeParams["sslmode"] from raw keyword input.
func ConfigToConnString(cfg *pgconn.Config) string {
	if cfg == nil {
		return ""
	}
	u := url.URL{Scheme: "postgres"}
	if cfg.User != "" {
		if cfg.Password != "" {
			u.User = url.UserPassword(cfg.User, cfg.Password)
		} else {
			u.User = url.User(cfg.User)
		}
	}
	host := cfg.Host
	if cfg.Port != 0 {
		u.Host = net.JoinHostPort(host, strconv.Itoa(int(cfg.Port)))
	} else {
		u.Host = host
	}
	if cfg.Database != "" {
		u.Path = "/" + cfg.Database
	}
	q := url.Values{}
	if len(cfg.RuntimeParams) > 0 {
		keys := make([]string, 0, len(cfg.RuntimeParams))
		for k := range cfg.RuntimeParams {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			q.Set(k, cfg.RuntimeParams[k])
		}
	}
	if _, ok := cfg.RuntimeParams["sslmode"]; !ok {
		// Lossy fallback: pgconn collapses sslmode into TLSConfig. If the
		// caller did not pre-populate cfg.RuntimeParams["sslmode"], fall
		// back to a nil/non-nil approximation. Strip/Compose pre-populate
		// for keyword inputs and bypass this path for URI inputs.
		if cfg.TLSConfig == nil {
			q.Set("sslmode", "disable")
		} else {
			q.Set("sslmode", "require")
		}
	}
	if encoded := q.Encode(); encoded != "" {
		u.RawQuery = encoded
	}
	return u.String()
}

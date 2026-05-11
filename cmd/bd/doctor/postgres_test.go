package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func writeMetadata(t *testing.T, beadsDir, body string) {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	t.Cleanup(clearResolveBeadsDirCache)
}

func TestIsPostgresBackend(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   bool
	}{
		{"no config", "", false},
		{"dolt backend", `{"backend":"dolt"}`, false},
		{"postgres backend", `{"backend":"postgres","postgres_dsn":"postgres://x@y/z"}`, true},
		{"empty backend defaults to dolt", `{"backend":""}`, false},
		{"unknown backend", `{"backend":"sqlite"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			beadsDir := filepath.Join(tmp, ".beads")
			if tc.config != "" {
				writeMetadata(t, beadsDir, tc.config)
			} else {
				if err := os.MkdirAll(beadsDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			}
			if got := IsPostgresBackend(beadsDir); got != tc.want {
				t.Errorf("IsPostgresBackend(%q) = %v, want %v", tc.config, got, tc.want)
			}
		})
	}
}

func TestRunPostgresHealthChecks_NotPostgresBackend(t *testing.T) {
	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	writeMetadata(t, beadsDir, `{"backend":"dolt"}`)

	checks := RunPostgresHealthChecks(tmp)
	if len(checks) != 6 {
		t.Fatalf("expected 6 N/A checks, got %d", len(checks))
	}

	expectedNames := []string{
		"Postgres Connection",
		"Postgres Schema Version",
		"Postgres Tables",
		"Postgres Activity",
		"Repo Fingerprint",
		"Referential Integrity",
	}
	for i, c := range checks {
		if c.Name != expectedNames[i] {
			t.Errorf("checks[%d].Name = %q, want %q", i, c.Name, expectedNames[i])
		}
		if c.Status != StatusOK {
			t.Errorf("checks[%d] (%s): want StatusOK for non-PG backend, got %s", i, c.Name, c.Status)
		}
		if !strings.Contains(c.Message, "N/A") {
			t.Errorf("checks[%d] (%s).Message = %q, want N/A message", i, c.Name, c.Message)
		}
	}
}

func TestRunPostgresHealthChecks_MissingDSN(t *testing.T) {
	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	// Postgres backend but no postgres_dsn → openPGConn returns descriptive
	// error, all 6 checks surface as errors.
	writeMetadata(t, beadsDir, `{"backend":"postgres"}`)

	checks := RunPostgresHealthChecks(tmp)
	if len(checks) != 6 {
		t.Fatalf("expected 6 checks, got %d", len(checks))
	}

	// First entry should be the actionable connection error.
	conn := checks[0]
	if conn.Name != "Postgres Connection" {
		t.Errorf("checks[0].Name = %q, want \"Postgres Connection\"", conn.Name)
	}
	if conn.Status != StatusError {
		t.Errorf("checks[0].Status = %q, want StatusError", conn.Status)
	}
	if !strings.Contains(conn.Detail, "postgres_dsn is empty") {
		t.Errorf("checks[0].Detail = %q, want it to mention empty postgres_dsn", conn.Detail)
	}
	if conn.Fix == "" {
		t.Errorf("checks[0].Fix is empty; want actionable next-step")
	}

	// Remaining checks (Schema/Tables/Activity/Repo Fingerprint/Referential
	// Integrity) should be skipped-with-detail when the connection failed.
	for i := 1; i < 6; i++ {
		if checks[i].Status != StatusError {
			t.Errorf("checks[%d] (%s): want StatusError (skipped after connect fail), got %s",
				i, checks[i].Name, checks[i].Status)
		}
		if !strings.Contains(checks[i].Message, "Skipped") {
			t.Errorf("checks[%d] (%s).Message = %q, want \"Skipped\"-style message",
				i, checks[i].Name, checks[i].Message)
		}
	}
}

func TestRunPostgresHealthChecks_UnparseableDSN(t *testing.T) {
	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	// Garbage that pgxpool.ParseConfig rejects. The classifier must NOT echo
	// the DSN itself (it may carry credentials in real-world misconfigs).
	writeMetadata(t, beadsDir, `{"backend":"postgres","postgres_dsn":"this is not a dsn"}`)

	checks := RunPostgresHealthChecks(tmp)
	conn := checks[0]
	if conn.Status != StatusError {
		t.Errorf("Status = %q, want StatusError", conn.Status)
	}
	if strings.Contains(conn.Detail, "this is not a dsn") {
		t.Errorf("Detail must not echo the raw DSN, got %q", conn.Detail)
	}
}

func TestClassifyPGError_AuthCodes(t *testing.T) {
	cases := []struct {
		name string
		code string
		want string
	}{
		{"invalid_authorization_specification", "28000", "authentication failed"},
		{"invalid_password", "28P01", "authentication failed"},
		{"invalid_catalog_name", "3D000", "database does not exist"},
		{"cannot_connect_now", "57P03", "starting up"},
		{"other_code", "42P01", "server error 42P01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pgErr := &pgconn.PgError{Code: tc.code, Message: "ignored"}
			got := classifyPGError(pgErr)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Errorf("classifyPGError(%s) = %v, want substring %q", tc.code, got, tc.want)
			}
		})
	}
}

func TestClassifyPGError_NetworkPatterns(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantSub string
	}{
		{"timeout via context", "dial: context deadline exceeded", "", "connect timeout"},
		{"io timeout", "read tcp: i/o timeout", "", "connect timeout"},
		{"connection refused", "dial tcp 127.0.0.1:5432: connection refused", "", "connection refused"},
		{"no such host", "dial tcp: lookup foo: no such host", "", "host unreachable"},
		{"no route to host", "dial tcp 10.0.0.1:5432: no route to host", "", "host unreachable"},
		{"password failed (no pgError)", "FATAL: password authentication failed for user", "", "authentication failed"},
		{"TLS issue", "tls: handshake failure", "", "TLS handshake failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPGError(errors.New(tc.raw))
			if got == nil {
				t.Fatalf("classifyPGError(%q) = nil, want non-nil", tc.raw)
			}
			if !strings.Contains(got.Error(), tc.wantSub) {
				t.Errorf("classifyPGError(%q) = %q, want substring %q", tc.raw, got.Error(), tc.wantSub)
			}
		})
	}
}

func TestClassifyPGError_PreservesOriginalOnUnknown(t *testing.T) {
	// An unrecognized network/server error should pass through. The doctor
	// caller treats this as the catch-all Detail.
	raw := errors.New("some thoroughly unexpected pgx-level error")
	got := classifyPGError(raw)
	if got == nil {
		t.Fatal("classifyPGError(nil) should not return nil for non-nil input")
	}
	if got.Error() != raw.Error() {
		t.Errorf("classifyPGError unknown = %q, want %q (pass-through)", got, raw)
	}
}

func TestClassifyPGError_NilInput(t *testing.T) {
	if got := classifyPGError(nil); got != nil {
		t.Errorf("classifyPGError(nil) = %v, want nil", got)
	}
}

func TestRoundDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{2 * time.Minute, "2m"},
		{59 * time.Minute, "59m"},
		{90 * time.Minute, "1h"},
		{23 * time.Hour, "23h"},
		{25 * time.Hour, "1d"},
		{72 * time.Hour, "3d"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := roundDuration(tc.d); got != tc.want {
				t.Errorf("roundDuration(%s) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

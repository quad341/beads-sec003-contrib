package postgres

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestWrapErrAuthFailureEnvUnset verifies that a 28P01 (or 28000) pgconn error
// is wrapped as *ErrAuthFailure with the env-unset hint when
// BEADS_POSTGRES_PASSWORD is not set in the environment.
func TestWrapErrAuthFailureEnvUnset(t *testing.T) {
	t.Setenv("BEADS_POSTGRES_PASSWORD", "")

	for _, code := range []string{"28P01", "28000"} {
		t.Run("code="+code, func(t *testing.T) {
			pgErr := &pgconn.PgError{Code: code, Message: "password authentication failed for user \"alice\""}
			wrapped := wrapErr("server version check", pgErr)

			var typed *ErrAuthFailure
			if !errors.As(wrapped, &typed) {
				t.Fatalf("expected *ErrAuthFailure, got %T: %v", wrapped, wrapped)
			}
			if typed.PGCode != code {
				t.Errorf("PGCode: got %q, want %q", typed.PGCode, code)
			}
			if typed.Op != "server version check" {
				t.Errorf("Op: got %q, want %q", typed.Op, "server version check")
			}
			if !typed.EnvUnset {
				t.Error("EnvUnset: got false, want true")
			}
			if !strings.Contains(wrapped.Error(), "BEADS_POSTGRES_PASSWORD is not set") {
				t.Errorf("expected env-unset hint in error: %q", wrapped.Error())
			}
		})
	}
}

// TestWrapErrAuthFailureEnvSet verifies the env-set hint variant.
func TestWrapErrAuthFailureEnvSet(t *testing.T) {
	t.Setenv("BEADS_POSTGRES_PASSWORD", "somepassword")

	pgErr := &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}
	wrapped := wrapErr("server version check", pgErr)

	var typed *ErrAuthFailure
	if !errors.As(wrapped, &typed) {
		t.Fatalf("expected *ErrAuthFailure, got %T: %v", wrapped, wrapped)
	}
	if typed.EnvUnset {
		t.Error("EnvUnset: got true, want false")
	}
	if !strings.Contains(wrapped.Error(), "BEADS_POSTGRES_PASSWORD is set but the password was rejected") {
		t.Errorf("expected env-set hint in error: %q", wrapped.Error())
	}
}

// TestWrapErrNonAuthErrorUnchanged verifies that non-auth pgconn errors fall
// through to the existing string-form wrapping.
func TestWrapErrNonAuthErrorUnchanged(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42P01", Message: "relation \"foo\" does not exist"}
	wrapped := wrapErr("read foo", pgErr)

	var typed *ErrAuthFailure
	if errors.As(wrapped, &typed) {
		t.Fatalf("did not expect *ErrAuthFailure for non-auth code, got: %v", wrapped)
	}
	got := wrapped.Error()
	if !strings.HasPrefix(got, "postgres: read foo: ") {
		t.Errorf("expected legacy string-form prefix, got: %q", got)
	}
	if !strings.Contains(got, "relation \"foo\" does not exist") {
		t.Errorf("expected upstream message preserved, got: %q", got)
	}
}

// TestErrAuthFailureUnwrap verifies that errors.Unwrap returns the redacted
// cause.
func TestErrAuthFailureUnwrap(t *testing.T) {
	t.Setenv("BEADS_POSTGRES_PASSWORD", "")
	pgErr := &pgconn.PgError{Code: "28P01", Message: "password authentication failed for user \"alice\""}
	wrapped := wrapErr("server version check", pgErr)

	cause := errors.Unwrap(wrapped)
	if cause == nil {
		t.Fatal("expected non-nil Unwrap")
	}
	if !strings.Contains(cause.Error(), "password authentication failed") {
		t.Errorf("expected cause to retain auth-failure text, got: %q", cause.Error())
	}
}

// TestErrAuthFailureSentinelRedacted verifies the DSN scrubbing still applies
// to the auth-failure path — a sentinel password embedded in the upstream
// error must not leak into the wrapped form.
func TestErrAuthFailureSentinelRedacted(t *testing.T) {
	t.Setenv("BEADS_POSTGRES_PASSWORD", "")
	const sentinel = "RIDICULOUSLY_SECRET_SENTINEL_DO_NOT_LOG"
	pgErr := &pgconn.PgError{
		Code:    "28P01",
		Message: fmt.Sprintf("auth failed for postgres://alice:%s@host/db", sentinel),
	}
	wrapped := wrapErr("server version check", pgErr)

	if strings.Contains(wrapped.Error(), sentinel) {
		t.Errorf("sentinel password leaked into wrapped error: %q", wrapped.Error())
	}
}

// TestWrapErrNilInputReturnsNil pins the existing nil-passthrough behavior.
func TestWrapErrNilInputReturnsNil(t *testing.T) {
	if got := wrapErr("any op", nil); got != nil {
		t.Errorf("wrapErr(op, nil) = %v, want nil", got)
	}
}

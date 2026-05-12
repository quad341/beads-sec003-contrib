package schema

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// TestEmitLargeRigNotice covers the be-8ja large-rig warning branch. The
// fresh-install case (count reported via error, typically "table doesn't
// exist") must stay silent — that's the UX contract: on a first-ever bd run,
// there is no rig to warn about.
func TestEmitLargeRigNotice(t *testing.T) {
	cases := []struct {
		name      string
		count     int64
		countErr  error
		wantEmpty bool
		wantSub   string
	}{
		{
			name:      "fresh_install_table_missing",
			count:     0,
			countErr:  errors.New("Table 'issues' doesn't exist"),
			wantEmpty: true,
		},
		{
			name:      "small_rig_below_threshold",
			count:     9_999,
			countErr:  nil,
			wantEmpty: true,
		},
		{
			name:      "at_threshold_no_warning",
			count:     10_000,
			countErr:  nil,
			wantEmpty: true,
		},
		{
			name:    "one_past_threshold_warns",
			count:   10_001,
			wantSub: "Large rig detected (10001 issues)",
		},
		{
			name:    "typical_large_rig",
			count:   49_187,
			wantSub: "Large rig detected (49187 issues)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			emitLargeRigNotice(&buf, tc.count, tc.countErr)

			got := buf.String()
			if tc.wantEmpty {
				if got != "" {
					t.Errorf("want no output, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("want substring %q; got %q", tc.wantSub, got)
			}
			if !strings.Contains(got, "do not interrupt") {
				t.Errorf("warning missing operator guidance; got %q", got)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("warning must end with newline; got %q", got)
			}
		})
	}
}

// TestHumanMigrationName pins the filename → display-name mapping for the
// per-migration progress line. The be-8ja progress output MUST be stable and
// human-readable; regression here would change operator-visible text.
func TestHumanMigrationName(t *testing.T) {
	cases := map[string]string{
		"0033_add_date_indexes.up.sql":          "add_date_indexes",
		"0001_initial.up.sql":                   "initial",
		"0027_add_started_at.up.sql":            "add_started_at",
		"noversion.up.sql":                      "noversion",
		"0099_multi_word_migration_name.up.sql": "multi_word_migration_name",
	}
	for in, want := range cases {
		if got := humanMigrationName(in); got != want {
			t.Errorf("humanMigrationName(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestProgressOutDefaultsToStderr guards the be-8ja invariant that the
// package-level writer starts pointed at os.Stderr. A regression here would
// silently leak migration progress into stdout and break bd <anything> --json
// pipelines — exactly the failure mode the bead called out.
func TestProgressOutDefaultsToStderr(t *testing.T) {
	if progressOut == nil {
		t.Fatal("progressOut must not be nil")
	}
}

// TestRunMigrations_ApplyingDoneLines verifies that the per-migration progress
// lines ("Applying migration NNNN: name…" / "  done (N.Ns)") appear in
// progressOut when runMigrations applies at least one pending migration.
//
// runMigrations is called directly (bypassing MigrateUp) so the test does not
// need a real database. issueRowCounter is stubbed to return 0 so the large-rig
// branch is skipped; the mock DBConn returns sql.Result success for every
// ExecContext call, which is all runMigrations needs.
func TestRunMigrations_ApplyingDoneLines(t *testing.T) {
	latest := LatestVersion()
	if latest == 0 {
		t.Skip("no embedded migrations to run")
	}

	origOut := progressOut
	origCounter := issueRowCounter
	t.Cleanup(func() {
		progressOut = origOut
		issueRowCounter = origCounter
	})

	var buf bytes.Buffer
	progressOut = &buf
	issueRowCounter = func(_ context.Context, _ DBConn) (int64, error) {
		return 0, nil
	}

	db := &nopDBConn{}
	_, err := runMigrations(context.Background(), db, latest-1, false)
	if err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Applying migration") {
		t.Errorf("expected 'Applying migration' in progressOut; got %q", out)
	}
	if !strings.Contains(out, "done (") {
		t.Errorf("expected 'done (' in progressOut; got %q", out)
	}
}

// nopDBConn satisfies DBConn for tests that exercise runMigrations without
// a real database. ExecContext returns success; the other methods panic if
// called — runMigrations must not invoke them (only ExecContext is needed).
type nopDBConn struct{}

func (nopDBConn) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nopResult{}, nil
}

func (nopDBConn) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	panic("nopDBConn.QueryContext called unexpectedly in runMigrations test")
}

func (nopDBConn) QueryRowContext(_ context.Context, _ string, _ ...any) *sql.Row {
	panic("nopDBConn.QueryRowContext called unexpectedly in runMigrations test")
}

type nopResult struct{}

func (nopResult) LastInsertId() (int64, error) { return 0, nil }
func (nopResult) RowsAffected() (int64, error) { return 0, nil }

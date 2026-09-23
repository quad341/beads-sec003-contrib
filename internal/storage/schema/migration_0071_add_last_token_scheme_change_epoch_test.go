package schema

import (
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/testutil"
)

// R20-n multi-bump mixed-trigger epoch survival (architect ruling, be-bo451;
// gastownhall/beads#6681; this slice: be-dnykf) adds one nullable column:
// store_epoch gains last_token_scheme_change_epoch INT. store_epoch
// (migration 0067) already carries epoch and bumped_reason, but
// bumped_reason is a singleton overwritten on every bump -- it cannot answer
// "when did the MOST RECENT token-scheme-change bump happen" once a store
// has bumped more than once under mixed triggers, which is exactly the
// question addressSurvivesTransitionInTx (internal/storage/issueops/epoch_cas.go)
// needs answered to anchor its retained-mapping check to the right epoch
// instead of to whichever trigger happened to bump last.

const migration0071Up = "0071_add_last_token_scheme_change_epoch.up.sql"
const migration0071Down = "0071_add_last_token_scheme_change_epoch.down.sql"

// TestLatestVersionIncludesMigration0071 (pinning LatestVersion() == 71) is
// superseded by TestLatestVersionIncludesMigration0072
// (migration_0072_add_expected_revision_records_test.go) now that 0072
// claims the next free slot -- only one such pin lives at a time, matching
// how this test itself already superseded 0070's.
//
// 0072 is R16's per-record CAS (be-x5jqd.3), not a continuation of this
// slice's own R20-n work -- it landed here because be-mw8o9's rebase onto
// current main fixed the stubbed CAS leg on top of this stack rather than
// mid-stack, to avoid re-opening the 0069/0070/0071 supersession chain.

// TestMigration0071AddsLastTokenSchemeChangeEpoch is a pure-Go, DB-independent
// check of the frozen migration bytes themselves — it runs even where no
// `dolt` binary is available.
//
// It pins the same guarded-PREPARE shape 0067/0068/0069 use and for the same
// reason: the guard is what makes a raw replay of this file onto an
// already-migrated store a no-op, and Dolt accepts no unprepared conditional
// ADD COLUMN. The cost is the same pre-2.3 CLI hazard those migrations carry,
// so this migration needs the same direct-DDL override in
// cliCompatibleMigrationSQL, policed by the same two guard tests
// (TestBundleMigrationsWithPreparedALTERAreOverriddenOrJustified and
// TestPreparedALTERSafeOnFreshBundleHasNoStaleEntries).
func TestMigration0071AddsLastTokenSchemeChangeEpoch(t *testing.T) {
	upSQL, err := MigrationSQL(migration0071Up)
	if err != nil {
		t.Fatalf("MigrationSQL(%s) error = %v, want the migration file to exist", migration0071Up, err)
	}
	for _, want := range []string{
		"ALTER TABLE store_epoch ADD COLUMN last_token_scheme_change_epoch INT",
		"COLUMN_NAME = 'last_token_scheme_change_epoch'",
		"@store_epoch_ltsce_needs_add",
	} {
		if !strings.Contains(upSQL, want) {
			t.Errorf("0071 up migration missing %q\nfull SQL:\n%s", want, upSQL)
		}
	}
	if !strings.Contains(strings.ToUpper(upSQL), "PREPARE STMT FROM @SQL") {
		t.Error("0071 up migration must keep its guarded PREPARE block — it is what makes a raw .up.sql replay onto an already-migrated store a no-op, and Dolt accepts no unprepared conditional ADD COLUMN. Unwrapping it also invalidates cliMigration0071AddLastTokenSchemeChangeEpoch.")
	}
	// The bundle override is what keeps the PREPARE above off the pre-2.3
	// CLI path. Assert it directly rather than trusting the two schema_test
	// assertions to stay pointed at this migration.
	const wantDirectDDL = "ALTER TABLE store_epoch ADD COLUMN last_token_scheme_change_epoch INT NULL;"
	if !strings.Contains(cliCompatibleMigrationSQL(migration0071Up, upSQL), wantDirectDDL) {
		t.Errorf("0071's CLI bundle substitute missing direct DDL %q", wantDirectDDL)
	}
	if cliSubstituteAssumesWispTables(migration0071Up) {
		t.Error("0071's CLI substitute touches only store_epoch, which has no wisps-side counterpart table — it must not be listed in cliSubstituteAssumesWispTables")
	}

	// down.sql files are not part of the embedded FS (only migrations/*.up.sql
	// is //go:embed'd — see mainSource.files), so unlike the up side above,
	// this reads straight from disk by package-relative path, matching
	// TestMigration0069AddsRemovedRestriction's precedent.
	downBytes, err := os.ReadFile("migrations/" + migration0071Down)
	if err != nil {
		t.Fatalf("read %s: %v, want the migration file to exist", migration0071Down, err)
	}
	downSQL := string(downBytes)
	for _, want := range []string{
		"ALTER TABLE store_epoch DROP COLUMN last_token_scheme_change_epoch",
		"COLUMN_NAME = 'last_token_scheme_change_epoch'",
	} {
		if !strings.Contains(downSQL, want) {
			t.Errorf("0071 down migration missing %q\nfull SQL:\n%s", want, downSQL)
		}
	}
	// Only migrations/*.up.sql is embedded into the CLI fresh bundle
	// (mainSource.files), so the pre-2.3 prepared-DDL hazard never reaches a
	// down migration and the guard is free — 0067's/0069's downs are the
	// precedent.
	if !strings.Contains(strings.ToUpper(downSQL), "PREPARE STMT FROM @SQL") {
		t.Error("0071 down migration must guard its DROP COLUMN the way the up migration guards its ADD COLUMN, so a partially-applied or already-rolled-back workspace rolls back safely")
	}
}

// TestMigration0071AddsLastTokenSchemeChangeEpochThroughDoltCLI applies the
// full migration bundle through a real `dolt` binary (skipped without one —
// see testutil.RequireDoltBinary) and checks the shape acceptance criteria a
// pure-Go SQL-text check cannot: actual column type/nullability as Dolt
// reports it, that the column stays NULL for a store that has never bumped
// under a token-scheme-change trigger, and that the same-statement
// left-to-right SET assignment order BumpEpochInTx depends on
// (last_token_scheme_change_epoch = epoch + 1 listed BEFORE epoch = epoch +
// 1) reads the PRE-increment epoch rather than double-bumping.
func TestMigration0071AddsLastTokenSchemeChangeEpochThroughDoltCLI(t *testing.T) {
	testutil.RequireDoltBinary(t)

	dir := t.TempDir()
	runDoltCommand(t, dir, "init", "--name", "test", "--email", "test@example.com")
	runDoltSQL(t, dir, AllMigrationsSQL())

	requireDoltColumnShape(t, dir, "store_epoch", "last_token_scheme_change_epoch", "int", "YES")

	runDoltSQL(t, dir, `INSERT INTO store_epoch (id, epoch) VALUES (1, 1)`)
	// id rides along as a non-NULL anchor column: queryDoltCSV trims the whole
	// `dolt sql -r csv` output before parsing, and a row whose ONLY selected
	// column is NULL renders as a bare blank line, which strings.TrimSpace
	// swallows when it's the trailing line — selecting id alongside keeps the
	// row's CSV line non-blank so it survives the trim (same gotcha
	// TestMigration0069AddsRemovedRestrictionThroughDoltCLI documents).
	rows := queryDoltCSV(t, dir, `SELECT id, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`)
	if len(rows) != 1 || rows[0]["last_token_scheme_change_epoch"] != "" {
		t.Fatalf("last_token_scheme_change_epoch before any token-scheme-change bump = %v, want NULL/empty", rows)
	}

	runDoltSQL(t, dir, `UPDATE store_epoch SET last_token_scheme_change_epoch = epoch + 1, epoch = epoch + 1, bumped_at = NOW(), bumped_reason = 'token-scheme-change' WHERE id = 1`)
	rows = queryDoltCSV(t, dir, `SELECT id, epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`)
	if len(rows) != 1 || rows[0]["epoch"] != "2" || rows[0]["last_token_scheme_change_epoch"] != "2" {
		t.Fatalf("store_epoch after a token-scheme-change bump = %v, want epoch=2 and last_token_scheme_change_epoch=2 (assignment order matters: the latter must read the PRE-increment epoch, not double-bump)", rows)
	}
}

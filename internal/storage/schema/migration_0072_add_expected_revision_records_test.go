package schema

import "testing"

// R16's per-record CAS (gastownhall/beads#5898 revision 9, this slice:
// be-x5jqd.3 / #6133) adds one brand-new table, expected_revision_records —
// see migrations/0072_add_expected_revision_records.up.sql for the full
// rationale. Unlike 0067/0068, this migration is a plain, unguarded
// CREATE TABLE IF NOT EXISTS with no ALTER and no PREPARE, so it needs none
// of those migrations' CLI-hazard-override tests (already covered, for every
// migration including this one, by TestBundleMigrationsWithPreparedALTERAreOverriddenOrJustified
// and TestAllMigrationsSQLUsesDirectDDLForKnownCLIIncompatibilities).

// TestLatestVersionIncludesMigration0072 pins the real next free slot this
// phase claims, superseding 0071's own version of this test (LatestVersion()
// moved from 71 to 72 the moment this migration file was added). Deliberately
// a hardcoded literal for the same reason 0071's was: LatestVersion()
// drifting to 72 for the wrong reason (an unrelated migration landing first)
// should still be caught by this test failing to explain why 72 is
// expected-revision-shaped. Slotted in at 72, not 69, because 69-71 were
// already claimed by be-dnykf's versioned-history stack (0069_add_removed_restriction,
// 0070_add_epoch_minted_addresses, 0071_add_last_token_scheme_change_epoch)
// by the time this leg's stub was fixed during the be-mw8o9 rebase onto
// current main.
func TestLatestVersionIncludesMigration0072(t *testing.T) {
	const want = 72
	if got := LatestVersion(); got != want {
		t.Fatalf("LatestVersion() = %d, want %d (expected_revision_records migration slot claimed by be-x5jqd.3)", got, want)
	}
}

package schema

// R20's epoch-transition enforcement (gastownhall/beads#5898 revision 9,
// this slice: be-x5jqd.4 / #6136) adds one brand-new table,
// epoch_minted_addresses -- see
// migrations/0070_add_epoch_minted_addresses.up.sql for the full rationale.
// This migration is a plain, unguarded
// CREATE TABLE IF NOT EXISTS with no ALTER and no PREPARE, so it needs none
// of 0067/0068's CLI-hazard-override tests (already covered, for every
// migration including this one, by TestBundleMigrationsWithPreparedALTERAreOverriddenOrJustified
// and TestAllMigrationsSQLUsesDirectDDLForKnownCLIIncompatibilities).

// TestLatestVersionIncludesMigration0070 (pinning LatestVersion() == 70) is
// superseded by TestLatestVersionIncludesMigration0071
// (migration_0071_add_last_token_scheme_change_epoch_test.go) now that 0071
// claims the next free slot -- only one such pin lives at a time, matching
// how this test itself already superseded 0069's.
//
// This slice originally claimed 0069 too. It was renumbered to 0070 because
// R7.1's removed_restriction migration -- a PARALLEL SIBLING of this branch,
// not an ancestor -- had also claimed 0069, and the two are now linearized:
// R7.1 keeps 0069 (it is published beneath gastownhall/beads#6661 and cannot
// move), this one takes the next contiguous slot. Two files sharing a
// version number panic checkNoDuplicateVersions at store open, before any
// command's RunE, which bricks every bd invocation rather than merely
// reddening a package -- see the same hazard on #6358 vs #6008.

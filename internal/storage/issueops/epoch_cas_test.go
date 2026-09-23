package issueops

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestStillServesInTxHonorsARetainedMapping pins R20-n's retained-mapping
// exception (gastownhall/beads#5898 revision 9): "a version that survives
// the transition keeps its address resolving ... under a change of token
// scheme, through a retained mapping from the prior-epoch address, which
// also reports the current one — so an epoch voids what the store lost,
// never what it still serves." Before this fix, StillServesInTx only ever
// checked whether the literal queried address's own row matched the
// current epoch, so a prior-epoch address whose id had already been
// carried forward into the current epoch (via CurrentAddressForInTx or a
// fresh MintUnderEpochInTx) still wrongly reported false.
func TestStillServesInTxHonorsARetainedMapping(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "still-serves-store"
	const oldAddress = "epch:still-serves-store:record-a:1"

	mock.ExpectQuery(`SELECT store_id, minted_id, minted_epoch FROM epoch_minted_addresses WHERE address = \?`).
		WithArgs(oldAddress).
		WillReturnRows(sqlmock.NewRows([]string{"store_id", "minted_id", "minted_epoch"}).AddRow(storeID, "record-a", 1))
	mock.ExpectQuery(`SELECT epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch", "last_token_scheme_change_epoch"}).AddRow(2, 2))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch = \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := StillServesInTx(context.Background(), tx, storeID, oldAddress)
	if err != nil {
		t.Fatalf("StillServesInTx: %v", err)
	}
	if !got {
		t.Fatal("StillServesInTx(a prior-epoch address whose id was carried forward) = false, want true (R20-n's retained-mapping exception)")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet retained-mapping SQL expectations: %v", err)
	}
}

// TestStillServesInTxReportsFalseWhenIDWasNotCarriedForward is the
// regression guard for the retained-mapping fix above: a prior-epoch
// address whose id was never re-minted at the current epoch stays not
// served, per R20-n's default (an epoch bump voids what the store lost).
func TestStillServesInTxReportsFalseWhenIDWasNotCarriedForward(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "still-serves-store"
	const oldAddress = "epch:still-serves-store:record-b:1"

	mock.ExpectQuery(`SELECT store_id, minted_id, minted_epoch FROM epoch_minted_addresses WHERE address = \?`).
		WithArgs(oldAddress).
		WillReturnRows(sqlmock.NewRows([]string{"store_id", "minted_id", "minted_epoch"}).AddRow(storeID, "record-b", 1))
	mock.ExpectQuery(`SELECT epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch", "last_token_scheme_change_epoch"}).AddRow(2, 2))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch = \?`).
		WithArgs(storeID, "record-b", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := StillServesInTx(context.Background(), tx, storeID, oldAddress)
	if err != nil {
		t.Fatalf("StillServesInTx: %v", err)
	}
	if got {
		t.Fatal("StillServesInTx(a prior-epoch address whose id was never carried forward) = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet not-carried-forward SQL expectations: %v", err)
	}
}

// TestResolveEpochInTxHonorsARetainedMapping mirrors the StillServesInTx
// case above for ResolveEpochInTx: a retained mapping resolves Live under
// the CURRENT epoch, not GoneReorganization, and Epoch names the current
// epoch (R20-n).
func TestResolveEpochInTxHonorsARetainedMapping(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "resolve-store"
	const oldAddress = "epch:resolve-store:record-a:1"

	mock.ExpectQuery(`SELECT store_id, minted_id, minted_epoch FROM epoch_minted_addresses WHERE address = \?`).
		WithArgs(oldAddress).
		WillReturnRows(sqlmock.NewRows([]string{"store_id", "minted_id", "minted_epoch"}).AddRow(storeID, "record-a", 1))
	mock.ExpectQuery(`SELECT epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch", "last_token_scheme_change_epoch"}).AddRow(2, 2))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch = \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := ResolveEpochInTx(context.Background(), tx, storeID, oldAddress)
	if err != nil {
		t.Fatalf("ResolveEpochInTx: %v", err)
	}
	if got.Restriction != EpochRestrictionLive {
		t.Errorf("ResolveEpochInTx(a prior-epoch address whose id was carried forward).Restriction = %v, want EpochRestrictionLive (R20-n's retained-mapping exception)", got.Restriction)
	}
	if got.Epoch == nil || *got.Epoch != 2 {
		t.Errorf("ResolveEpochInTx(a prior-epoch address whose id was carried forward).Epoch = %v, want the current epoch (2)", got.Epoch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet retained-mapping SQL expectations: %v", err)
	}
}

// TestAddressSurvivesTransitionInTxWithNoSchemeChangeSurvivesWhenUntouched
// pins R20-n's multi-bump generalization (architect ruling, be-bo451) at the
// addressSurvivesTransitionInTx level directly: with no token-scheme-change
// bump ever recorded (lastTokenSchemeChangeEpoch == nil, "k = none"),
// survival reduces to the original restore/destructive-reinit rule — no
// later mint since mintedEpoch — regardless of how many restore/reinit bumps
// happened in between.
func TestAddressSurvivesTransitionInTxWithNoSchemeChangeSurvivesWhenUntouched(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := addressSurvivesTransitionInTx(context.Background(), tx, storeID, "record-a", 1, 3, nil)
	if err != nil {
		t.Fatalf("addressSurvivesTransitionInTx: %v", err)
	}
	if !got {
		t.Fatal("addressSurvivesTransitionInTx(k=none, no later mint) = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

// TestAddressSurvivesTransitionInTxWithNoSchemeChangeFailsOnALaterMint is the
// k=none regression guard's negative twin.
func TestAddressSurvivesTransitionInTxWithNoSchemeChangeFailsOnALaterMint(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	got, err := addressSurvivesTransitionInTx(context.Background(), tx, storeID, "record-a", 1, 3, nil)
	if err != nil {
		t.Fatalf("addressSurvivesTransitionInTx: %v", err)
	}
	if got {
		t.Fatal("addressSurvivesTransitionInTx(k=none, a later mint exists) = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

// TestAddressSurvivesTransitionInTxTreatsASchemeChangeAtOrBeforeMintAsNone
// pins the boundary the architect ruling's "k = last_token_scheme_change_epoch
// IF non-NULL AND > mintedEpoch, else none" test draws: a scheme change that
// happened AT OR BEFORE this very mint is not a transition the address needs
// a bridge for (it was already minted under the post-change scheme), so it
// must fall through to the plain no-later-mint check, not the bridge check.
func TestAddressSurvivesTransitionInTxTreatsASchemeChangeAtOrBeforeMintAsNone(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	lastSchemeChange := 2
	got, err := addressSurvivesTransitionInTx(context.Background(), tx, storeID, "record-a", 2, 3, &lastSchemeChange)
	if err != nil {
		t.Fatalf("addressSurvivesTransitionInTx: %v", err)
	}
	if !got {
		t.Fatal("addressSurvivesTransitionInTx(last_token_scheme_change_epoch == mintedEpoch, no later mint) = false, want true: a scheme change AT OR BEFORE the mint is not a transition needing its own bridge")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v — a scheme change at or before mintedEpoch must not trigger a mintedIDHasAddressAtEpochInTx bridge check at all", err)
	}
}

// TestAddressSurvivesTransitionInTxSurvivesASchemeChangeBridgeThroughALaterRestore
// is architect-ruling scenario (a): bridged once, at the scheme-change
// epoch, then left untouched through a later restore. A rule anchored to
// "the latest bump's trigger" (the pre-R20-n behavior) would see the latest
// trigger here as restore and wrongly require no mint since mintedEpoch —
// but the bridge mint IS a mint since mintedEpoch, so that rule would
// wrongly report Gone. R20-n's fix anchors to the most recent
// token-scheme-change epoch specifically, however many other bumps came
// after it.
func TestAddressSurvivesTransitionInTxSurvivesASchemeChangeBridgeThroughALaterRestore(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch = \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	k := 2
	got, err := addressSurvivesTransitionInTx(context.Background(), tx, storeID, "record-a", 1, 3, &k)
	if err != nil {
		t.Fatalf("addressSurvivesTransitionInTx: %v", err)
	}
	if !got {
		t.Fatal("addressSurvivesTransitionInTx(bridged at the scheme-change epoch, then a later restore) = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

// TestAddressSurvivesTransitionInTxRequiresABridgeAtTheMostRecentSchemeChange
// is architect-ruling scenario (b): two token-scheme-change bumps with a
// restore in between, bridged only at the first. The bridge made for the
// first scheme change does not carry the address through the second, later
// one — each scheme change re-encodes addresses again, so a missing bridge
// at the most recent one is exactly as fatal as never having bridged at
// all, per R20-n's no-self-healing invariant. This is also the case a naive
// "any bump in range had a matching mint" generalization would get wrong in
// the other direction, by accepting the stale first-scheme-change bridge.
func TestAddressSurvivesTransitionInTxRequiresABridgeAtTheMostRecentSchemeChange(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch = \?`).
		WithArgs(storeID, "record-a", int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	k := 4
	got, err := addressSurvivesTransitionInTx(context.Background(), tx, storeID, "record-a", 1, 4, &k)
	if err != nil {
		t.Fatalf("addressSurvivesTransitionInTx: %v", err)
	}
	if got {
		t.Fatal("addressSurvivesTransitionInTx(bridged only at an earlier scheme change, not the most recent one) = true, want false: a missing bridge at the most recent scheme change is permanent")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v — this also pins the short-circuit: mintedIDHasLaterMintInTx must not run once the bridge check at k already fails", err)
	}
}

// TestAddressSurvivesTransitionInTxSurvivesRestoresOnBothSidesOfABridgedSchemeChange
// is architect-ruling scenario (c): a restore before AND a restore after a
// single, bridged scheme change. Restore never sets
// last_token_scheme_change_epoch, however many of them surround the one
// scheme change that does, so k anchors to the scheme change regardless of
// its position in the bump sequence.
func TestAddressSurvivesTransitionInTxSurvivesRestoresOnBothSidesOfABridgedSchemeChange(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch = \?`).
		WithArgs(storeID, "record-a", int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM epoch_minted_addresses WHERE store_id = \? AND minted_id = \? AND minted_epoch > \?`).
		WithArgs(storeID, "record-a", int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	k := 3
	got, err := addressSurvivesTransitionInTx(context.Background(), tx, storeID, "record-a", 1, 4, &k)
	if err != nil {
		t.Fatalf("addressSurvivesTransitionInTx: %v", err)
	}
	if !got {
		t.Fatal("addressSurvivesTransitionInTx(a leading and a trailing restore around a bridged scheme change) = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

// TestBumpEpochInTxSetsLastTokenSchemeChangeEpochBeforeIncrementingEpochInTheSameStatement
// pins the exact statement shape the bead's exit contract calls for: same
// transaction as the existing store_epoch UPDATE, with
// last_token_scheme_change_epoch's assignment listed BEFORE epoch's in one
// multi-column SET clause. MySQL/Dolt evaluates a multi-column SET
// left-to-right within a single statement, so listing
// last_token_scheme_change_epoch = epoch + 1 AFTER epoch = epoch + 1 would
// read the ALREADY-INCREMENTED epoch and double-bump it — this test's regex
// pins the ordering directly rather than relying on that evaluation-order
// behavior being exercised only incidentally by a value-based assertion.
func TestBumpEpochInTxSetsLastTokenSchemeChangeEpochBeforeIncrementingEpochInTheSameStatement(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch", "last_token_scheme_change_epoch"}).AddRow(2, nil))
	mock.ExpectExec(`UPDATE store_epoch SET last_token_scheme_change_epoch = epoch \+ 1, epoch = epoch \+ 1, bumped_at = \?, bumped_reason = \? WHERE id = 1`).
		WithArgs(sqlmock.AnyArg(), epochBumpReasonTokenSchemeChange).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch"}).AddRow(3))

	got, err := BumpEpochInTx(context.Background(), tx, storeID, epochBumpReasonTokenSchemeChange)
	if err != nil {
		t.Fatalf("BumpEpochInTx: %v", err)
	}
	if got != 3 {
		t.Fatalf("BumpEpochInTx(token-scheme-change) = %d, want 3", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v — last_token_scheme_change_epoch must be assigned before epoch in the same SET clause", err)
	}
}

// TestBumpEpochInTxLeavesLastTokenSchemeChangeEpochUntouchedOnARestoreBump is
// the regression guard proving a non-scheme-change bump's UPDATE statement
// text is unchanged: it must not assign last_token_scheme_change_epoch at
// all (not even a same-value no-op), matching the architect ruling's "left
// untouched on Restore/destructive-reinit" requirement.
func TestBumpEpochInTxLeavesLastTokenSchemeChangeEpochUntouchedOnARestoreBump(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)
	const storeID = "store"

	mock.ExpectQuery(`SELECT epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch", "last_token_scheme_change_epoch"}).AddRow(2, nil))
	mock.ExpectExec(`UPDATE store_epoch SET epoch = epoch \+ 1, bumped_at = \?, bumped_reason = \? WHERE id = 1`).
		WithArgs(sqlmock.AnyArg(), "restore").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT epoch FROM store_epoch WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"epoch"}).AddRow(3))

	got, err := BumpEpochInTx(context.Background(), tx, storeID, "restore")
	if err != nil {
		t.Fatalf("BumpEpochInTx: %v", err)
	}
	if got != 3 {
		t.Fatalf("BumpEpochInTx(restore) = %d, want 3", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v — a restore bump must not touch last_token_scheme_change_epoch", err)
	}
}

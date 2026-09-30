package dolt

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/steveyegge/beads/backend/conformance"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// TestRetentionContract wires this leg into the R20 retention contract.
// Phase 0 leaves every hook nil in every backend's fixture kit (architecture
// §12), so each case below skips by name; this file exists so
// TestEveryLegWiresEveryRoleContract counts this leg, and so the cases start
// running for real the moment this leg's fixture kit grows a non-nil hook.
func TestRetentionContract(t *testing.T) {
	ctx := context.Background()
	fixture := conformance.RetentionFixture{IssuePrefix: "rete"}

	t.Run("RemovalLeavesTheAddressAbleToAnswer", func(t *testing.T) {
		conformance.RunRemovalLeavesTheAddressAbleToAnswer(t, ctx, fixture)
	})
	t.Run("RemovalReasonIsRetentionErasureOrReorganizationDistinctly", func(t *testing.T) {
		conformance.RunRemovalReasonIsRetentionErasureOrReorganizationDistinctly(t, ctx, fixture)
	})
	t.Run("RemovalReportsTheSurvivingRetainedWindow", func(t *testing.T) {
		conformance.RunRemovalReportsTheSurvivingRetainedWindow(t, ctx, fixture)
	})
	t.Run("RemovalNeverReassignsASurvivingAddress", func(t *testing.T) {
		conformance.RunRemovalNeverReassignsASurvivingAddress(t, ctx, fixture)
	})
	t.Run("GoneIsDistinguishableFromUnknown", func(t *testing.T) {
		conformance.RunGoneIsDistinguishableFromUnknown(t, ctx, fixture)
	})
	t.Run("AnAddressNeverResolvesToADifferentState", func(t *testing.T) {
		conformance.RunAnAddressNeverResolvesToADifferentState(t, ctx, fixture)
	})
	t.Run("ADestructiveOperationEnumeratesAffectedAddressesFirst", func(t *testing.T) {
		conformance.RunADestructiveOperationEnumeratesAffectedAddressesFirst(t, ctx, fixture)
	})
	t.Run("AHoldPreventsRemovalAndReportsInRetainedBounds", func(t *testing.T) {
		conformance.RunAHoldPreventsRemovalAndReportsInRetainedBounds(t, ctx, fixture)
	})
	t.Run("ForcingAHeldRemovalRecordsWhoWhenWhy", func(t *testing.T) {
		conformance.RunForcingAHeldRemovalRecordsWhoWhenWhy(t, ctx, fixture)
	})
	t.Run("EveryRetentionAnswerNamesItsProducingStore", func(t *testing.T) {
		conformance.RunEveryRetentionAnswerNamesItsProducingStore(t, ctx, fixture)
	})
	t.Run("AStoreWithNoLineageKnowledgeAnswersUnknownNotGone", func(t *testing.T) {
		conformance.RunAStoreWithNoLineageKnowledgeAnswersUnknownNotGone(t, ctx, fixture)
	})
	t.Run("AStoreThatRemovesStateStillAnswersGoneDurably", func(t *testing.T) {
		conformance.RunAStoreThatRemovesStateStillAnswersGoneDurably(t, ctx, fixture)
	})
	t.Run("ErasureMintsACorrectedVersionRatherThanEditingInPlace", func(t *testing.T) {
		conformance.RunErasureMintsACorrectedVersionRatherThanEditingInPlace(t, ctx, fixture)
	})
}

// TestEpochContract wires this leg into the R20 epoch contract (R20-m/n).
// Unlike TestRetentionContract above, every EpochFixture hook is real here:
// store_epoch is a plain table this leg's *DoltStore already owns
// (CurrentEpoch/BumpEpoch, store.go), so there is no Phase-0 reason to leave
// it nil.
func TestEpochContract(t *testing.T) {
	fixture, ctx, cleanup := newDoltEpochFixture(t, "epch")
	defer cleanup()

	t.Run("AnEpochBumpIsTriggeredOnlyByRestoreReinitOrSchemeChange", func(t *testing.T) {
		conformance.RunAnEpochBumpIsTriggeredOnlyByRestoreReinitOrSchemeChange(t, ctx, fixture)
	})
	t.Run("EpochBumpVoidsOnlyAddressesOfVersionsNoLongerServed", func(t *testing.T) {
		// A skip is green, so without this check the voiding half of R20-n could
		// go unexercised on this leg with nothing failing: the case skips itself
		// whenever StillServes answers the same for every address. This leg can
		// make one version stop being served, so a skip here is a failure.
		defer func() {
			if t.Skipped() {
				t.Error("R20-n skipped on this leg: its fixture must make one minted version stop being served (StillServes false) while another stays served, or the voiding half of the contract never runs")
			}
		}()
		conformance.RunEpochBumpVoidsOnlyAddressesOfVersionsNoLongerServed(t, ctx, fixture)
	})
}

// doltEpochAddress encodes an EpochFixture Address as "<issueID>@<revision>".
// No existing convention to reuse: ExpectedRevisionFixture (the other
// contract with an Address-shaped concept) leaves every hook nil in every
// backend as of this writing (expected_revision_contract_test.go), so this
// encoding is local to this file, not a shared one — duplicated, not
// extracted, in the embeddeddolt and uow fixture files for the same reason
// DualWriteFixture's per-leg closures are not shared: three call sites don't
// justify a shared abstraction, and each leg's read path differs enough
// (plain *sql.DB here, a pinned kit connection in embeddeddolt, RawSQLUseCase
// in uow) that a shared helper would need its own indirection anyway.
func doltEpochAddress(issueID string, revision int64) conformance.Address {
	return conformance.Address(fmt.Sprintf("%s@%d", issueID, revision))
}

func parseDoltEpochAddress(address conformance.Address) (issueID string, revision int64, err error) {
	issueID, revisionStr, ok := strings.Cut(string(address), "@")
	if !ok {
		return "", 0, fmt.Errorf("malformed epoch address %q: want issueID@revision", address)
	}
	revision, err = strconv.ParseInt(revisionStr, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("malformed epoch address %q: %w", address, err)
	}
	return issueID, revision, nil
}

// newDoltEpochFixture wires conformance.EpochFixture to a real *DoltStore,
// mirroring newDoltDualWriteFixture's shape (setupTestStore/testContext/
// VersionedHistoryConfigurer/cleanup composition). Versioned history must be
// on: MintUnderEpoch mints through store.CreateIssue, and issues.
// current_revision (what CurrentAddressFor re-reads) only advances when
// RecordVersionInTx is active.
//
// StillServes/Resolve share ONE underlying criterion — row-existence in
// issue_versions for the exact (issueID, revision) the address names — which
// is deliberate, not a shortcut: nothing in this phase ever removes an
// issue_versions row, so every address minted by MintUnderEpoch answers
// "still served" for as long as the issue exists. R20-n's voids case
// (RunEpochBumpVoidsOnlyAddressesOfVersionsNoLongerServed) needs one minted
// address to still be served and the other not, to exercise both halves of
// the contract; this fixture cannot manufacture that contrast (there is no
// real erasure/reorganization mechanism yet to make one specific version
// stop being served), so that case hits its own pre-built
// "servesA == servesB" skip branch here. That is the honest answer for a
// backend whose store_epoch is real but whose retention/erasure machinery is
// not, not a gap to paper over.
func newDoltEpochFixture(t *testing.T, prefix string) (conformance.EpochFixture, context.Context, func()) {
	t.Helper()
	store, storeCleanup := setupTestStore(t)
	ctx, cancel := testContext(t)
	configurer, ok := any(store).(storage.VersionedHistoryConfigurer)
	if !ok {
		cancel()
		storeCleanup()
		t.Fatalf("%T does not implement storage.VersionedHistoryConfigurer", store)
	}
	configurer.SetVersionedHistoryEnabled(true)

	currentAddressFor := func(ctx context.Context, issueID string) (conformance.Address, error) {
		var revision int64
		if err := store.db.QueryRowContext(ctx,
			`SELECT current_revision FROM issues WHERE id = ?`, issueID,
		).Scan(&revision); err != nil {
			return "", fmt.Errorf("read current_revision for %s: %w", issueID, err)
		}
		return doltEpochAddress(issueID, revision), nil
	}

	fixture := conformance.EpochFixture{
		IssuePrefix: prefix,
		CurrentEpoch: func(ctx context.Context, storeID string) (int, error) {
			return store.CurrentEpoch(ctx)
		},
		BumpEpoch: func(ctx context.Context, storeID string, trigger conformance.EpochBumpTrigger) (int, error) {
			return store.BumpEpoch(ctx, trigger.String())
		},
		MintUnderEpoch: func(ctx context.Context, storeID, id string) (conformance.Address, error) {
			if err := store.CreateIssue(ctx, &types.Issue{
				ID: id, Title: "t-" + id, IssueType: types.TypeTask, Status: types.StatusOpen,
			}, "actor"); err != nil {
				return "", err
			}
			return currentAddressFor(ctx, id)
		},
		StillServes: func(ctx context.Context, storeID string, address conformance.Address) (bool, error) {
			issueID, revision, err := parseDoltEpochAddress(address)
			if err != nil {
				return false, err
			}
			var count int
			if err := store.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM issue_versions WHERE issue_id = ? AND revision = ?`, issueID, revision,
			).Scan(&count); err != nil {
				return false, err
			}
			return count > 0, nil
		},
		Resolve: func(ctx context.Context, storeID string, address conformance.Address) (conformance.RetentionAnswer, error) {
			issueID, revision, err := parseDoltEpochAddress(address)
			if err != nil {
				return conformance.RetentionAnswer{}, err
			}
			var revisionCount int
			if err := store.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM issue_versions WHERE issue_id = ? AND revision = ?`, issueID, revision,
			).Scan(&revisionCount); err != nil {
				return conformance.RetentionAnswer{}, fmt.Errorf("resolve %s: count exact revision: %w", address, err)
			}
			if revisionCount > 0 {
				return conformance.RetentionAnswer{Restriction: conformance.RestrictionLive, ProducingStore: storeID}, nil
			}
			var issueVersionCount int
			if err := store.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM issue_versions WHERE issue_id = ?`, issueID,
			).Scan(&issueVersionCount); err != nil {
				return conformance.RetentionAnswer{}, fmt.Errorf("resolve %s: count issue versions: %w", address, err)
			}
			if issueVersionCount == 0 {
				return conformance.RetentionAnswer{Restriction: conformance.RestrictionUnknown, ProducingStore: storeID}, nil
			}
			// Reachable once a real erasure/reorganization mechanism can drop
			// one specific revision while leaving the issue's other versions
			// in place — not yet, but the answer shape is correct today: name
			// the epoch the caller must have bumped past to get here.
			epoch, err := store.CurrentEpoch(ctx)
			if err != nil {
				return conformance.RetentionAnswer{}, fmt.Errorf("resolve %s: current epoch: %w", address, err)
			}
			return conformance.RetentionAnswer{
				Restriction:    conformance.RestrictionGoneReorganization,
				ProducingStore: storeID,
				Epoch:          &epoch,
			}, nil
		},
		CurrentAddressFor: func(ctx context.Context, storeID string, oldAddress conformance.Address) (conformance.Address, error) {
			issueID, _, err := parseDoltEpochAddress(oldAddress)
			if err != nil {
				return "", err
			}
			return currentAddressFor(ctx, issueID)
		},
	}
	return fixture, ctx, func() {
		configurer.SetVersionedHistoryEnabled(false)
		cancel()
		storeCleanup()
	}
}

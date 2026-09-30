//go:build cgo

package embeddeddolt_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/steveyegge/beads/backend/conformance"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
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
// store_epoch is a plain table this leg's *EmbeddedDoltStore already owns
// (CurrentEpoch/BumpEpoch, version_control.go), so there is no Phase-0 reason
// to leave it nil.
func TestEpochContract(t *testing.T) {
	skipUnlessEmbeddedDolt(t)
	te := newTestEnv(t, "epch")
	ctx := t.Context()
	fixture := newEmbeddedDoltEpochFixture(t, te, "epch")

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

// embeddedEpochAddress/parseEmbeddedEpochAddress duplicate the dolt leg's
// "<issueID>@<revision>" encoding locally rather than sharing it — see the
// dolt leg's doltEpochAddress doc comment for why three call sites don't
// justify extracting this into backend/conformance.
func embeddedEpochAddress(issueID string, revision int64) conformance.Address {
	return conformance.Address(fmt.Sprintf("%s@%d", issueID, revision))
}

func parseEmbeddedEpochAddress(address conformance.Address) (issueID string, revision int64, err error) {
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

// newEmbeddedDoltEpochFixture wires conformance.EpochFixture to a real
// *EmbeddedDoltStore, mirroring newEmbeddedDualWriteFixture's shape (te.store,
// VersionedHistoryConfigurer, newEmbeddedRoleFixtureKit's QueryScalar for raw
// reads). Unlike that fixture's Mutate/CurrentRevision hooks, CurrentEpoch and
// BumpEpoch here call the real te.store.CurrentEpoch/BumpEpoch production
// methods directly rather than reading/writing store_epoch through
// kit.QueryScalar: the whole point of R20-m's contract is to validate those
// two production methods, not to re-derive their answer independently.
//
// StillServes/Resolve/CurrentAddressFor mirror the dolt leg's fixture -- see
// its doc comment for how BumpEpoch(Restore) gives R20-n's voids case its one
// no-longer-served address. The removal itself runs through removeVersionRaw
// (version_removal_test.go): this package's tests reach the store only through
// its public surface, so a short-lived raw connection does the write, the same
// way the kit does its reads.
func newEmbeddedDoltEpochFixture(t *testing.T, te *testEnv, prefix string) conformance.EpochFixture {
	t.Helper()
	store := te.store
	configurer, ok := any(store).(storage.VersionedHistoryConfigurer)
	if !ok {
		t.Fatalf("%T does not implement storage.VersionedHistoryConfigurer", store)
	}
	configurer.SetVersionedHistoryEnabled(true)

	kit := newEmbeddedRoleFixtureKit(te, prefix)

	currentAddressFor := func(ctx context.Context, issueID string) (conformance.Address, error) {
		var revision int64
		if err := kit.QueryScalar(ctx, "SELECT current_revision FROM issues WHERE id = ?", []any{issueID}, &revision); err != nil {
			return "", fmt.Errorf("read current_revision for %s: %w", issueID, err)
		}
		return embeddedEpochAddress(issueID, revision), nil
	}

	// loseNewestMintUnder soft-removes the newest still-live version minted
	// under closingEpoch, if any -- the restore model the dolt leg's fixture
	// documents. change_at orders "newest"; issue_id and revision only break a
	// tie deterministically.
	loseNewestMintUnder := func(ctx context.Context, closingEpoch int) error {
		var issueID string
		var revision int64
		err := kit.QueryScalar(ctx,
			`SELECT issue_id, revision FROM issue_versions
			  WHERE epoch = ? AND removed_at IS NULL
			  ORDER BY change_at DESC, issue_id DESC, revision DESC LIMIT 1`,
			[]any{closingEpoch}, &issueID, &revision)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // nothing was minted under the closing epoch, so the restore loses nothing
		}
		if err != nil {
			return fmt.Errorf("find newest version minted under epoch %d: %w", closingEpoch, err)
		}
		return removeVersionRaw(ctx, te, issueID, revision, issueops.VersionRemovalReasonReorganization)
	}

	return conformance.EpochFixture{
		IssuePrefix: prefix,
		CurrentEpoch: func(ctx context.Context, storeID string) (int, error) {
			return store.CurrentEpoch(ctx)
		},
		BumpEpoch: func(ctx context.Context, storeID string, trigger conformance.EpochBumpTrigger) (int, error) {
			closing, err := store.CurrentEpoch(ctx)
			if err != nil {
				return 0, err
			}
			epoch, err := store.BumpEpoch(ctx, trigger.String())
			if err != nil {
				return 0, err
			}
			if trigger == conformance.EpochBumpTriggerRestore {
				if err := loseNewestMintUnder(ctx, closing); err != nil {
					return 0, fmt.Errorf("model the restore losing a version: %w", err)
				}
			}
			return epoch, nil
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
			issueID, revision, err := parseEmbeddedEpochAddress(address)
			if err != nil {
				return false, err
			}
			var count int
			if err := kit.QueryScalar(ctx,
				"SELECT COUNT(*) FROM issue_versions WHERE issue_id = ? AND revision = ? AND removed_at IS NULL",
				[]any{issueID, revision}, &count); err != nil {
				return false, err
			}
			return count > 0, nil
		},
		Resolve: func(ctx context.Context, storeID string, address conformance.Address) (conformance.RetentionAnswer, error) {
			issueID, revision, err := parseEmbeddedEpochAddress(address)
			if err != nil {
				return conformance.RetentionAnswer{}, err
			}
			var liveRevisionCount int
			if err := kit.QueryScalar(ctx,
				"SELECT COUNT(*) FROM issue_versions WHERE issue_id = ? AND revision = ? AND removed_at IS NULL",
				[]any{issueID, revision}, &liveRevisionCount); err != nil {
				return conformance.RetentionAnswer{}, fmt.Errorf("resolve %s: count live exact revision: %w", address, err)
			}
			if liveRevisionCount > 0 {
				return conformance.RetentionAnswer{Restriction: conformance.RestrictionLive, ProducingStore: storeID}, nil
			}
			var issueVersionCount int
			if err := kit.QueryScalar(ctx,
				"SELECT COUNT(*) FROM issue_versions WHERE issue_id = ?",
				[]any{issueID}, &issueVersionCount); err != nil {
				return conformance.RetentionAnswer{}, fmt.Errorf("resolve %s: count issue versions: %w", address, err)
			}
			if issueVersionCount == 0 {
				return conformance.RetentionAnswer{Restriction: conformance.RestrictionUnknown, ProducingStore: storeID}, nil
			}
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
			issueID, _, err := parseEmbeddedEpochAddress(oldAddress)
			if err != nil {
				return "", err
			}
			return currentAddressFor(ctx, issueID)
		},
	}
}

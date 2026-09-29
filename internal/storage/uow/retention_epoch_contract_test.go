package uow

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/backend/conformance"
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
// store_epoch is a plain table this leg reaches through RawSQLUseCase inside
// RunTx/RunTxRead, so there is no Phase-0 reason to leave it nil.
func TestEpochContract(t *testing.T) {
	ctx := context.Background()
	fixture := newUOWEpochFixture(t, ctx, "epch")

	t.Run("AnEpochBumpIsTriggeredOnlyByRestoreReinitOrSchemeChange", func(t *testing.T) {
		conformance.RunAnEpochBumpIsTriggeredOnlyByRestoreReinitOrSchemeChange(t, ctx, fixture)
	})
	t.Run("EpochBumpVoidsOnlyAddressesOfVersionsNoLongerServed", func(t *testing.T) {
		conformance.RunEpochBumpVoidsOnlyAddressesOfVersionsNoLongerServed(t, ctx, fixture)
	})
}

// uowEpochAddress/parseUOWEpochAddress duplicate the dolt leg's
// "<issueID>@<revision>" encoding locally rather than sharing it — see the
// dolt leg's doltEpochAddress doc comment for why three call sites don't
// justify extracting this into backend/conformance.
func uowEpochAddress(issueID string, revision int64) conformance.Address {
	return conformance.Address(fmt.Sprintf("%s@%d", issueID, revision))
}

func parseUOWEpochAddress(address conformance.Address) (issueID string, revision int64, err error) {
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

// uowCurrentEpochInTx replicates issueops.CurrentEpochInTx's seed-on-read
// logic (SELECT; if the shared row is missing, INSERT IGNORE it at 1 and
// SELECT again) against uw.RawSQLUseCase() rather than calling that function
// directly: domain.RawSQLUseCase returns a *domain.RawSQLResult, not the
// *sql.Rows/*sql.Row shape issueops.DBTX needs, so this leg's fixture must
// re-express the same SQL semantics through its own transactional surface
// instead of sharing the dolt/embeddeddolt helper. Called from both
// RunTxRead (CurrentEpoch) and from inside a RunTxResult attempt (BumpEpoch),
// so it takes a UnitOfWork directly rather than opening its own transaction.
func uowCurrentEpochInTx(ctx context.Context, uw UnitOfWork) (int, error) {
	result, err := uw.RawSQLUseCase().Query(ctx, "SELECT epoch FROM store_epoch WHERE id = 1")
	if err != nil {
		return 0, fmt.Errorf("store epoch: read: %w", err)
	}
	if len(result.Rows) == 0 {
		if _, err := uw.RawSQLUseCase().Exec(ctx, "INSERT IGNORE INTO store_epoch (id, epoch) VALUES (1, 1)"); err != nil {
			return 0, fmt.Errorf("store epoch: seed: %w", err)
		}
		result, err = uw.RawSQLUseCase().Query(ctx, "SELECT epoch FROM store_epoch WHERE id = 1")
		if err != nil {
			return 0, fmt.Errorf("store epoch: reread after seed: %w", err)
		}
	}
	if len(result.Rows) != 1 {
		return 0, fmt.Errorf("store epoch: query returned %d rows, want 1", len(result.Rows))
	}
	var epoch int
	if err := scanRawSQLValue(&epoch, result.Rows[0][0]); err != nil {
		return 0, fmt.Errorf("store epoch: scan: %w", err)
	}
	return epoch, nil
}

// newUOWEpochFixture wires conformance.EpochFixture directly to RunTx/
// RunTxRead + RawSQLUseCase, deliberately NOT touching the frozen
// roleFixtureKit (bd-kue5t) beyond reusing its CreateIssue/QueryScalar for the
// parts that are ordinary reads: CurrentEpoch and BumpEpoch replicate
// issueops.CurrentEpochInTx/BumpEpochInTx's SQL rather than calling them,
// because RawSQLUseCase cannot satisfy issueops.DBTX (see
// uowCurrentEpochInTx's doc comment). BumpEpoch commits through
// RunTxResult with a real, non-empty commit message — RunTxRead never calls
// uw.Commit, so using it here would silently roll the bump back.
//
// StillServes/Resolve/CurrentAddressFor and the "voids" case's unavoidable
// skip mirror the dolt leg's fixture exactly -- see its doc comment for why
// that skip is the honest terminal state here too (nothing yet removes an
// issue_versions row on any backend).
func newUOWEpochFixture(t *testing.T, ctx context.Context, prefix string) conformance.EpochFixture {
	t.Helper()
	provider := newUOWVersionedHistoryProvider(t, ctx, prefix, true)
	kit := newUOWRoleFixtureKit(provider, prefix)

	currentAddressFor := func(ctx context.Context, issueID string) (conformance.Address, error) {
		var revision int64
		if err := kit.QueryScalar(ctx, "SELECT current_revision FROM issues WHERE id = ?", []any{issueID}, &revision); err != nil {
			return "", fmt.Errorf("read current_revision for %s: %w", issueID, err)
		}
		return uowEpochAddress(issueID, revision), nil
	}

	return conformance.EpochFixture{
		IssuePrefix: prefix,
		CurrentEpoch: func(ctx context.Context, storeID string) (int, error) {
			return RunTxRead(ctx, provider, func(ctx context.Context, uw UnitOfWork) (int, error) {
				return uowCurrentEpochInTx(ctx, uw)
			})
		},
		BumpEpoch: func(ctx context.Context, storeID string, trigger conformance.EpochBumpTrigger) (int, error) {
			return RunTxResult(ctx, provider, func(ctx context.Context, uw UnitOfWork) (int, string, error) {
				if _, err := uowCurrentEpochInTx(ctx, uw); err != nil {
					return 0, "", fmt.Errorf("store epoch: bump: ensure seeded: %w", err)
				}
				// TEMPORARY RED (mol-tdd-build be-kt083 / be-s5rvc): dropped
				// "epoch = epoch + 1," to match the same deliberate regression in
				// issueops.BumpEpochInTx (this leg reimplements the statement rather
				// than calling that helper — see uowCurrentEpochInTx's doc comment).
				// Restored verbatim for GREEN.
				if _, err := uw.RawSQLUseCase().Exec(ctx,
					"UPDATE store_epoch SET bumped_at = ?, bumped_reason = ? WHERE id = 1",
					time.Now().UTC(), trigger.String(),
				); err != nil {
					return 0, "", fmt.Errorf("store epoch: bump: %w", err)
				}
				epoch, err := uowCurrentEpochInTx(ctx, uw)
				if err != nil {
					return 0, "", fmt.Errorf("store epoch: bump: reread: %w", err)
				}
				return epoch, fmt.Sprintf("bd: store epoch bump (%s)", trigger.String()), nil
			})
		},
		MintUnderEpoch: func(ctx context.Context, storeID, id string) (conformance.Address, error) {
			// This leg's CreateIssue goes through domain.IssueUseCase.CreateIssue
			// (kit.CreateIssue below), which validates an explicit ID's prefix
			// against the configured issue_prefix when both are set
			// (domain/issue.go's validateExplicitIDPrefix) -- unlike the
			// dolt/embeddeddolt direct legs, whose store-level CreateIssue does
			// not run that check. So this fixture must qualify the bare
			// "record-a"/"record-b" id the conformance case passes with this
			// fixture's own prefix, matching the convention every other fixture
			// in this package uses (e.g.
			// TestUOWCreateWithLabelAndEdgeMintsPerRepositoryWrite_KnownDivergence's
			// prefix + "-target").
			qualifiedID := prefix + "-" + id
			if err := kit.CreateIssue(ctx, &types.Issue{
				ID: qualifiedID, Title: "t-" + qualifiedID, IssueType: types.TypeTask, Status: types.StatusOpen,
			}, "actor"); err != nil {
				return "", err
			}
			return currentAddressFor(ctx, qualifiedID)
		},
		StillServes: func(ctx context.Context, storeID string, address conformance.Address) (bool, error) {
			issueID, revision, err := parseUOWEpochAddress(address)
			if err != nil {
				return false, err
			}
			var count int
			if err := kit.QueryScalar(ctx,
				"SELECT COUNT(*) FROM issue_versions WHERE issue_id = ? AND revision = ?",
				[]any{issueID, revision}, &count); err != nil {
				return false, err
			}
			return count > 0, nil
		},
		Resolve: func(ctx context.Context, storeID string, address conformance.Address) (conformance.RetentionAnswer, error) {
			issueID, revision, err := parseUOWEpochAddress(address)
			if err != nil {
				return conformance.RetentionAnswer{}, err
			}
			var revisionCount int
			if err := kit.QueryScalar(ctx,
				"SELECT COUNT(*) FROM issue_versions WHERE issue_id = ? AND revision = ?",
				[]any{issueID, revision}, &revisionCount); err != nil {
				return conformance.RetentionAnswer{}, fmt.Errorf("resolve %s: count exact revision: %w", address, err)
			}
			if revisionCount > 0 {
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
			epoch, err := RunTxRead(ctx, provider, func(ctx context.Context, uw UnitOfWork) (int, error) {
				return uowCurrentEpochInTx(ctx, uw)
			})
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
			issueID, _, err := parseUOWEpochAddress(oldAddress)
			if err != nil {
				return "", err
			}
			return currentAddressFor(ctx, issueID)
		},
	}
}

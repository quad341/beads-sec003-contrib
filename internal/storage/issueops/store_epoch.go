package issueops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// store_epoch is the single shared row (id = 1) RecordVersionInTx already
// reads on every mint (see version_history.go's lazy-seed comment on that
// read). This file adds the other half: bumping that same row, and reading
// it outside a minting transaction, for the R20 epoch contract
// (backend/conformance.EpochFixture; design R20-m/R20-n).
//
// EpochBumpReasonXxx spells the same vocabulary as
// backend/conformance.EpochBumpTrigger.String() (restore /
// destructive-reinit / token-scheme-change), duplicated here rather than
// imported: production code must not depend on a test-support package, and
// this package has no other reason to know about the conformance suite.
const (
	EpochBumpReasonRestore           = "restore"
	EpochBumpReasonDestructiveReinit = "destructive-reinit"
	EpochBumpReasonTokenSchemeChange = "token-scheme-change"
)

// CurrentEpochInTx reads store_epoch's shared row, seeding it at 1 the first
// time anything asks (mirrors RecordVersionInTx's own read exactly, so a
// store that has never minted a version and never bumped an epoch still
// answers 1 rather than an error). Safe to call from either a read-only or a
// read-write tx: the seed insert is idempotent (INSERT IGNORE) and only fires
// when the row is genuinely absent.
func CurrentEpochInTx(ctx context.Context, tx DBTX) (int, error) {
	var epoch int
	err := tx.QueryRowContext(ctx, "SELECT epoch FROM store_epoch WHERE id = 1").Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		if _, seedErr := tx.ExecContext(ctx, "INSERT IGNORE INTO store_epoch (id, epoch) VALUES (1, 1)"); seedErr != nil {
			return 0, fmt.Errorf("store epoch: seed: %w", seedErr)
		}
		err = tx.QueryRowContext(ctx, "SELECT epoch FROM store_epoch WHERE id = 1").Scan(&epoch)
	}
	if err != nil {
		return 0, fmt.Errorf("store epoch: read: %w", err)
	}
	return epoch, nil
}

// BumpEpochInTx advances store_epoch by exactly one and returns the new
// value, recording when and why (bumped_at, bumped_reason). reason should be
// one of the EpochBumpReasonXxx constants above, but this function does not
// validate it -- the vocabulary is closed by convention (its only callers are
// the store-level BumpEpoch methods), not by a check here.
//
// Callers of BumpEpochInTx MUST NOT stage/commit the store_epoch table (Dolt
// DOLT_ADD/DOLT_COMMIT) from INSIDE the same tx this runs in: doing so
// builds the Dolt commit from the transaction's BEGIN-time snapshot rather
// than what it just wrote, silently reverting the bump -- and because
// store_epoch is append-only-by-convention (every row's history matters,
// like issue_versions), a reverted row is never naturally rewritten the way
// an ordinary issues-table row would be on the next mutation. The store-level
// BumpEpoch methods (dolt, embeddeddolt) commit this function's SQL
// transaction FIRST, then stage+commit the working set as a SEPARATE later
// step. This function only ever runs the SQL half.
func BumpEpochInTx(ctx context.Context, tx DBTX, reason string) (int, error) {
	// Ensure the row exists before the UPDATE below: an UPDATE that matches
	// zero rows because store_epoch was never seeded would silently no-op
	// (SQL does not error on a no-match UPDATE), leaving epoch unbumped with
	// no signal. CurrentEpochInTx's seed-on-read makes the row's existence an
	// invariant from here on.
	if _, err := CurrentEpochInTx(ctx, tx); err != nil {
		return 0, fmt.Errorf("store epoch: bump: ensure seeded: %w", err)
	}

	// TEMPORARY RED (mol-tdd-build be-kt083 / be-s5rvc): dropped "epoch = epoch + 1,"
	// on purpose so RunAnEpochBumpIsTriggeredOnlyByRestoreReinitOrSchemeChange
	// fails for real (bumped_at/bumped_reason advance, epoch does not) instead
	// of skipping or passing immediately against the already-written store-level
	// CurrentEpoch/BumpEpoch wiring. Restored verbatim for GREEN.
	if _, err := tx.ExecContext(ctx,
		"UPDATE store_epoch SET bumped_at = ?, bumped_reason = ? WHERE id = 1",
		time.Now().UTC(), reason,
	); err != nil {
		return 0, fmt.Errorf("store epoch: bump: %w", err)
	}

	epoch, err := CurrentEpochInTx(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("store epoch: bump: reread: %w", err)
	}
	return epoch, nil
}

package issueops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// This file implements R20 epoch-transition enforcement (gastownhall/beads#5898
// revision 9, this slice: be-x5jqd.4 / #6136): a store-wide epoch generation
// counter (store_epoch, migration 0067) plus a durable record of the
// addresses minted under each generation (epoch_minted_addresses, migration
// 0069), used to answer whether a previously-minted address is still served
// by the store's current epoch. It adds no RetentionFixture/R17 resolve,
// remove, hold, force-remove, erase, or mint logic — R20 epoch reasoning is
// evaluated entirely on its own (out of scope for this slice).
//
// ALL THREE LEGS SHARE THIS BODY, the same way CompareAndSetMetadataKeyInTx
// and R16's CompareAndSetVersionInTx do: the two Dolt-backed stores wrap it
// in their own transaction, and the unit-of-work provider reaches it through
// its own leg-specific adapter. Each leg's own contract-test file is the
// translation boundary to and from backend/conformance's
// Address/EpochBumpTrigger/RetentionAnswer vocabulary — this file knows
// nothing about the conformance package.
//
// ADDRESSES ARE NEVER RECOMPUTED FROM store_epoch. Each is a deterministic
// token over (storeID, id, the epoch current at mint time) — see
// epochAddress — persisted once at mint and never rewritten in place: a
// later mint of the same id under a bumped epoch produces a DIFFERENT
// address and a new row, so a superseded address's row survives to answer
// StillServes/Resolve as "no longer served" after the epoch moves on.

// EpochRestriction is this file's own local answer vocabulary for R20 —
// deliberately not backend/conformance.Restriction (see package doc above).
// It omits GoneRetention/GoneErasure: those are RetentionFixture/R17
// outcomes this slice does not produce.
type EpochRestriction int

const (
	EpochRestrictionLive EpochRestriction = iota
	EpochRestrictionGoneReorganization
	EpochRestrictionUnknown
)

// EpochResolveResult is ResolveEpochInTx's answer. Epoch is non-nil exactly
// when the answer required comparing an epoch (Live and GoneReorganization);
// it is nil for Unknown, which never looks store_epoch up at all.
type EpochResolveResult struct {
	Restriction    EpochRestriction
	ProducingStore string
	Epoch          *int
}

// ErrEpochAddressNotFound means the address named was never minted by
// storeID — distinct from an ordinary "gone" answer, which requires having
// minted the address in the first place.
var ErrEpochAddressNotFound = errors.New("epoch CAS: address not minted by this store")

// epochAddress is a minted address: a deterministic token over storeID, id,
// and the epoch current at mint time, so re-minting the same id under the
// same epoch always reproduces the same address.
func epochAddress(storeID, id string, epoch int) string {
	return fmt.Sprintf("epch:%s:%s:%d", storeID, id, epoch)
}

// ensureStoreEpochRow lazily initializes store_epoch's singleton row
// (migration 0067; the table starts empty) to epoch 1 on first use, and
// reports the current epoch together with last_token_scheme_change_epoch
// (migration 0071, R20-n / architect ruling be-bo451) either way: nil when
// no token-scheme-change bump has ever landed (bumped_reason alone cannot
// answer this once a store has bumped more than once under mixed triggers,
// since it is a singleton overwritten on every bump — see
// addressSurvivesTransitionInTx).
func ensureStoreEpochRow(ctx context.Context, tx DBTX) (int, *int, error) {
	var epoch int
	var lastSchemeChange sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT epoch, last_token_scheme_change_epoch FROM store_epoch WHERE id = 1`).Scan(&epoch, &lastSchemeChange)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO store_epoch (id, epoch) VALUES (1, 1)`); err != nil {
			return 0, nil, fmt.Errorf("initialize store_epoch: %w", err)
		}
		return 1, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("read store_epoch: %w", err)
	}
	if !lastSchemeChange.Valid {
		return epoch, nil, nil
	}
	v := int(lastSchemeChange.Int64)
	return epoch, &v, nil
}

// epochMintedAddress is one row as read from epoch_minted_addresses.
type epochMintedAddress struct {
	storeID     string
	mintedID    string
	mintedEpoch int
}

// readEpochMintedAddressInTx reads address's row, if any. found is false
// (with a zero-value row and nil error) when address was never minted —
// distinguishing "not found" from an actual query error is the caller's
// job, the same way readExpectedRevisionRowInTx's callers do it.
func readEpochMintedAddressInTx(ctx context.Context, tx DBTX, address string) (epochMintedAddress, bool, error) {
	var row epochMintedAddress
	err := tx.QueryRowContext(ctx,
		`SELECT store_id, minted_id, minted_epoch FROM epoch_minted_addresses WHERE address = ?`, address,
	).Scan(&row.storeID, &row.mintedID, &row.mintedEpoch)
	if errors.Is(err, sql.ErrNoRows) {
		return epochMintedAddress{}, false, nil
	}
	if err != nil {
		return epochMintedAddress{}, false, fmt.Errorf("epoch CAS: read minted address %s: %w", address, err)
	}
	return row, true, nil
}

// upsertEpochMintedAddressInTx writes address's row via an explicit
// exists-check branch (the read already ran in the caller; exists says
// which branch to take) rather than ON DUPLICATE KEY UPDATE, matching
// upsertExpectedRevisionRowInTx's precedent.
func upsertEpochMintedAddressInTx(ctx context.Context, tx DBTX, address, storeID, mintedID string, mintedEpoch int, exists bool) error {
	now := time.Now().UTC()
	if exists {
		if _, err := tx.ExecContext(ctx,
			`UPDATE epoch_minted_addresses SET store_id = ?, minted_id = ?, minted_epoch = ?, minted_at = ? WHERE address = ?`,
			storeID, mintedID, mintedEpoch, now, address,
		); err != nil {
			return fmt.Errorf("epoch CAS: update minted address %s: %w", address, err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO epoch_minted_addresses (address, store_id, minted_id, minted_epoch, minted_at) VALUES (?, ?, ?, ?, ?)`,
		address, storeID, mintedID, mintedEpoch, now,
	); err != nil {
		return fmt.Errorf("epoch CAS: insert minted address %s: %w", address, err)
	}
	return nil
}

// CurrentEpochInTx reports storeID's current epoch generation. store_epoch
// has no store_id column: migration 0067 established it as one physical
// counter per database, matching production reality (one logical store per
// database already) — storeID is accepted here only to satisfy
// EpochFixture's hook signature.
func CurrentEpochInTx(ctx context.Context, tx DBTX, storeID string) (int, error) {
	epoch, _, err := ensureStoreEpochRow(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("epoch CAS: current epoch for %s: %w", storeID, err)
	}
	return epoch, nil
}

// BumpEpochInTx advances storeID's epoch generation by one and records why
// (R20-a: a bump is triggered only by restore, destructive-reinit, or
// token-scheme-change — the leg adapter converts the fixture's
// conformance.EpochBumpTrigger to this plain string via trigger.String(),
// keeping that vocabulary out of this file per the package doc above).
//
// On a token-scheme-change bump only (R20-n / architect ruling be-bo451),
// the same statement also sets last_token_scheme_change_epoch to the
// post-increment epoch, so a later addressSurvivesTransitionInTx call can
// anchor its retained-mapping check to the epoch of the most recent
// token-scheme-change bump specifically, not to whichever trigger happened
// to bump last. last_token_scheme_change_epoch = epoch + 1 MUST be listed
// BEFORE epoch = epoch + 1 in the SET clause: MySQL/Dolt evaluates a
// multi-column SET left-to-right within one statement, so the reverse order
// would read the already-incremented epoch and double-bump it. A restore or
// destructive-reinit bump leaves last_token_scheme_change_epoch untouched —
// not even a same-value no-op — matching the ruling's "no retained-mapping
// mechanism for those triggers" (be-mnw3c Q2).
func BumpEpochInTx(ctx context.Context, tx DBTX, storeID, reason string) (int, error) {
	if _, _, err := ensureStoreEpochRow(ctx, tx); err != nil {
		return 0, fmt.Errorf("epoch CAS: bump epoch for %s: %w", storeID, err)
	}
	query := `UPDATE store_epoch SET epoch = epoch + 1, bumped_at = ?, bumped_reason = ? WHERE id = 1`
	if reason == epochBumpReasonTokenSchemeChange {
		query = `UPDATE store_epoch SET last_token_scheme_change_epoch = epoch + 1, epoch = epoch + 1, bumped_at = ?, bumped_reason = ? WHERE id = 1`
	}
	if _, err := tx.ExecContext(ctx, query, time.Now().UTC(), reason); err != nil {
		return 0, fmt.Errorf("epoch CAS: bump epoch for %s: %w", storeID, err)
	}
	var newEpoch int
	if err := tx.QueryRowContext(ctx, `SELECT epoch FROM store_epoch WHERE id = 1`).Scan(&newEpoch); err != nil {
		return 0, fmt.Errorf("epoch CAS: read epoch after bump for %s: %w", storeID, err)
	}
	return newEpoch, nil
}

// MintUnderEpochInTx mints id's address under storeID's CURRENT epoch,
// deterministically (epochAddress): minting the same id again under the
// same epoch reproduces the same address and is an idempotent no-op upsert
// of the same row.
func MintUnderEpochInTx(ctx context.Context, tx DBTX, storeID, id string) (string, error) {
	epoch, _, err := ensureStoreEpochRow(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("epoch CAS: mint %s under epoch for %s: %w", id, storeID, err)
	}
	address := epochAddress(storeID, id, epoch)
	_, found, err := readEpochMintedAddressInTx(ctx, tx, address)
	if err != nil {
		return "", err
	}
	if err := upsertEpochMintedAddressInTx(ctx, tx, address, storeID, id, epoch, found); err != nil {
		return "", err
	}
	return address, nil
}

// epochBumpReasonTokenSchemeChange is BumpEpochInTx's reason string for a
// token-scheme-change trigger (conformance.EpochBumpTrigger.String()) — the
// only trigger addressSurvivesTransitionInTx treats as having a
// retained-mapping mechanism (architect ruling, be-mnw3c Q2).
const epochBumpReasonTokenSchemeChange = "token-scheme-change"

// mintedIDHasAddressAtEpochInTx reports whether mintedID has an address
// minted under storeID at epoch — the token-scheme-change branch of R20-n's
// survival rule (addressSurvivesTransitionInTx): a version carried forward
// by a retained mapping gets a fresh mint AT the current epoch, so a second,
// later row sharing the same id is that carry-forward act, not a
// derivation. mintedID is never recomputed from a bumped store_epoch (see
// the package doc above).
func mintedIDHasAddressAtEpochInTx(ctx context.Context, tx DBTX, storeID, mintedID string, epoch int) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM epoch_minted_addresses WHERE store_id = ? AND minted_id = ? AND minted_epoch = ?`,
		storeID, mintedID, epoch,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("epoch CAS: check retained mapping for %s at epoch %d: %w", mintedID, epoch, err)
	}
	return count > 0, nil
}

// mintedIDHasLaterMintInTx reports whether mintedID has a row minted at an
// epoch later than afterEpoch — the restore/destructive-reinit branch of
// R20-n's survival rule (addressSurvivesTransitionInTx). Those triggers have
// no retained-mapping concept (architect ruling, be-mnw3c Q2): a later mint
// of the same id means its content was superseded by something else after
// this address's own epoch, not carried forward, so the absence of any
// later mint is what proves an address is still untouched.
func mintedIDHasLaterMintInTx(ctx context.Context, tx DBTX, storeID, mintedID string, afterEpoch int) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM epoch_minted_addresses WHERE store_id = ? AND minted_id = ? AND minted_epoch > ?`,
		storeID, mintedID, afterEpoch,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("epoch CAS: check later mint for %s after epoch %d: %w", mintedID, afterEpoch, err)
	}
	return count > 0, nil
}

// addressSurvivesTransitionInTx decides R20-n survival for a minted address
// whose row is not at the current epoch (architect ruling be-bo451,
// generalizing the single-bump rule to multi-bump mixed-trigger histories).
//
// Let k = lastTokenSchemeChangeEpoch (store_epoch.last_token_scheme_change_epoch,
// read by ensureStoreEpochRow) if it is non-nil AND greater than mintedEpoch,
// else "none" — a scheme change at or before the mint is not a transition
// this address needs a bridge for, since it was already minted under the
// post-change scheme.
//
//   - k = none: survival is "no later mint exists" for mintedID since its
//     own mintedEpoch — the pre-R20-n single-bump rule, generalized to
//     ignore any number of intervening restore/destructive-reinit bumps,
//     neither of which has a retained-mapping mechanism (architect ruling,
//     be-mnw3c Q2).
//   - k exists: survival requires BOTH a bridge — mintedID has an address
//     minted at epoch k, the deliberate carry-forward act — AND no later
//     mint since k. A missing bridge at k is checked first and short-circuits
//     the rest: it is permanent and does not self-heal on a later bump, so
//     mintedIDHasLaterMintInTx must not run once the bridge check already
//     fails (an earlier scheme change's bridge never substitutes for a
//     missing one at the MOST RECENT scheme change).
func addressSurvivesTransitionInTx(ctx context.Context, tx DBTX, storeID, mintedID string, mintedEpoch int, _ int, lastTokenSchemeChangeEpoch *int) (bool, error) {
	hasBridgeEpoch := lastTokenSchemeChangeEpoch != nil && *lastTokenSchemeChangeEpoch > mintedEpoch
	if !hasBridgeEpoch {
		hasLaterMint, err := mintedIDHasLaterMintInTx(ctx, tx, storeID, mintedID, mintedEpoch)
		if err != nil {
			return false, err
		}
		return !hasLaterMint, nil
	}
	k := *lastTokenSchemeChangeEpoch
	bridged, err := mintedIDHasAddressAtEpochInTx(ctx, tx, storeID, mintedID, k)
	if err != nil {
		return false, err
	}
	if !bridged {
		return false, nil
	}
	hasLaterMint, err := mintedIDHasLaterMintInTx(ctx, tx, storeID, mintedID, k)
	if err != nil {
		return false, err
	}
	return !hasLaterMint, nil
}

// StillServesInTx reports whether address is still served under storeID's
// CURRENT epoch: an address minted under an earlier epoch is no longer
// served once the epoch has moved past it, UNLESS R20-n's survival rule
// says its underlying id survives the transition that moved it
// (addressSurvivesTransitionInTx), even though the row itself is never
// deleted (ResolveEpochInTx must still be able to answer for it). An
// address from a different store, or one never minted, is not served
// either.
func StillServesInTx(ctx context.Context, tx DBTX, storeID, address string) (bool, error) {
	row, found, err := readEpochMintedAddressInTx(ctx, tx, address)
	if err != nil {
		return false, err
	}
	if !found || row.storeID != storeID {
		return false, nil
	}
	epoch, lastSchemeChange, err := ensureStoreEpochRow(ctx, tx)
	if err != nil {
		return false, fmt.Errorf("epoch CAS: still serves %s for %s: %w", address, storeID, err)
	}
	if row.mintedEpoch == epoch {
		return true, nil
	}
	survives, err := addressSurvivesTransitionInTx(ctx, tx, storeID, row.mintedID, row.mintedEpoch, epoch, lastSchemeChange)
	if err != nil {
		return false, fmt.Errorf("epoch CAS: still serves %s for %s: %w", address, storeID, err)
	}
	return survives, nil
}

// ResolveEpochInTx answers R20's epoch-only restriction for address: Live
// while its minting epoch is still current OR R20-n's survival rule says
// its id survives the transition that moved it
// (addressSurvivesTransitionInTx), GoneReorganization once the epoch has
// moved past it with no survival (a reorganization, not a retention or
// erasure outcome; RetentionFixture/R17 states are out of scope here), and
// Unknown for an address this store never minted. ProducingStore is always
// storeID: this file has no lineage/replica model to attribute a foreign
// store to (unlike RetentionFixture's cross-store answers).
func ResolveEpochInTx(ctx context.Context, tx DBTX, storeID, address string) (EpochResolveResult, error) {
	row, found, err := readEpochMintedAddressInTx(ctx, tx, address)
	if err != nil {
		return EpochResolveResult{}, err
	}
	if !found || row.storeID != storeID {
		return EpochResolveResult{Restriction: EpochRestrictionUnknown, ProducingStore: storeID}, nil
	}
	epoch, lastSchemeChange, err := ensureStoreEpochRow(ctx, tx)
	if err != nil {
		return EpochResolveResult{}, fmt.Errorf("epoch CAS: resolve %s for %s: %w", address, storeID, err)
	}
	if row.mintedEpoch == epoch {
		return EpochResolveResult{Restriction: EpochRestrictionLive, ProducingStore: storeID, Epoch: &epoch}, nil
	}
	survives, err := addressSurvivesTransitionInTx(ctx, tx, storeID, row.mintedID, row.mintedEpoch, epoch, lastSchemeChange)
	if err != nil {
		return EpochResolveResult{}, fmt.Errorf("epoch CAS: resolve %s for %s: %w", address, storeID, err)
	}
	if survives {
		return EpochResolveResult{Restriction: EpochRestrictionLive, ProducingStore: storeID, Epoch: &epoch}, nil
	}
	return EpochResolveResult{Restriction: EpochRestrictionGoneReorganization, ProducingStore: storeID, Epoch: &epoch}, nil
}

// CurrentAddressForInTx re-mints oldAddress's underlying id under storeID's
// CURRENT epoch, giving callers a live address to move to once oldAddress
// stops being served. oldAddress must be one storeID has actually minted.
func CurrentAddressForInTx(ctx context.Context, tx DBTX, storeID, oldAddress string) (string, error) {
	row, found, err := readEpochMintedAddressInTx(ctx, tx, oldAddress)
	if err != nil {
		return "", err
	}
	if !found || row.storeID != storeID {
		return "", fmt.Errorf("epoch CAS: current address for %s: %w: %s", storeID, ErrEpochAddressNotFound, oldAddress)
	}
	return MintUnderEpochInTx(ctx, tx, storeID, row.mintedID)
}

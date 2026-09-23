-- Migration 0071: R20-n multi-bump mixed-trigger epoch survival (architect
-- ruling be-bo451; gastownhall/beads#6681), this slice: be-dnykf.
--
-- store_epoch already carries epoch and bumped_reason (migration 0067), but
-- bumped_reason is a singleton overwritten on every bump -- it cannot answer
-- "when did the MOST RECENT token-scheme-change bump happen" once a store
-- has bumped more than once under mixed triggers (restore, reinit,
-- token-scheme-change, in any order). addressSurvivesTransitionInTx
-- (internal/storage/issueops/epoch_cas.go) needs that answer to anchor its
-- retained-mapping check to the epoch of the last token-scheme-change bump,
-- not to whichever trigger happened to bump last.
--
-- last_token_scheme_change_epoch INT is the one column this migration adds:
-- nullable, and NULL for every existing row and every store that has never
-- bumped under the token-scheme-change trigger -- BumpEpochInTx sets it to
-- the post-increment epoch only when reason ==
-- epochBumpReasonTokenSchemeChange, leaving it untouched on a restore or
-- reinit bump. A NULL value (or a value at or below the address's own
-- minted epoch) means "no scheme-change bump since this address was
-- minted," in which case addressSurvivesTransitionInTx falls back to the
-- pre-R20-n single-bump rule; a value above the minted epoch means that
-- epoch is the one and only bridge the address must be re-minted at to
-- survive, permanently -- a missing bridge at that epoch does not
-- self-heal on a later bump.
--
-- Guarded the same way 0067/0068/0069 guard their ADD COLUMNs (see 0067's
-- header for the full explanation): no MariaDB-only IF NOT EXISTS on Dolt
-- 2.2.3's ADD COLUMN, so an INFORMATION_SCHEMA probe + PREPARE is the only
-- replay-safe shape. Needs a CLI-bundle direct-DDL override
-- (cliMigration0071AddLastTokenSchemeChangeEpoch in cli_migrations.go), the
-- same dolthub/dolt#11345 escape hatch 0067/0068/0069 use, guarded by
-- TestBundleMigrationsWithPreparedALTERAreOverriddenOrJustified.
--
-- store_epoch has no wisps-side counterpart table (it is a single main-plane
-- singleton, migration 0067), so cliSubstituteAssumesWispTables does not
-- apply here either, matching 0070.
SET @store_epoch_ltsce_needs_add = (
    SELECT IF(COUNT(*) = 0, 1, 0)
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'store_epoch'
      AND COLUMN_NAME = 'last_token_scheme_change_epoch'
);
SET @sql = IF(@store_epoch_ltsce_needs_add = 1,
    'ALTER TABLE store_epoch ADD COLUMN last_token_scheme_change_epoch INT',
    'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

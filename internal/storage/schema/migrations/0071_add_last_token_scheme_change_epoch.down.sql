-- Reverse of 0071: drop store_epoch.last_token_scheme_change_epoch.
--
-- Guarded on INFORMATION_SCHEMA the same way 0067/0068/0069's downs are, so
-- a partially-applied or already-rolled-back workspace rolls back safely.
-- Only migrations/*.up.sql is embedded into the CLI fresh bundle, so the
-- PREPARE hazard (cli_prepared_ddl.go) never reaches this file.
SET @store_epoch_ltsce_has = (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'store_epoch'
      AND COLUMN_NAME = 'last_token_scheme_change_epoch'
);
SET @sql = IF(@store_epoch_ltsce_has > 0,
    'ALTER TABLE store_epoch DROP COLUMN last_token_scheme_change_epoch',
    'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

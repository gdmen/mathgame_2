-- Backfill grade_level from difficulty for older rows. Guarded on the disabled
-- column still existing: migration 46 dropped it, so on a fresh bootstrap (whose
-- generated problems table never had disabled) this no-ops on an empty table
-- instead of erroring on the disabled reference. grade_level itself is later
-- dropped in migration 40, so skipping the backfill on a fresh DB is harmless.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'disabled') > 0,
  'UPDATE problems SET grade_level = GREATEST(1, LEAST(8, CAST(FLOOR((difficulty - 4) / 2) AS SIGNED))) WHERE grade_level = 0 AND generator != ''heuristic_0.0'' AND disabled = 0',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

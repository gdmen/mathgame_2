-- grade_level is fully removed: the bitmap describes the allowed problem
-- shapes and target_difficulty is the adaptive lever within that envelope.
-- No settings translation: existing users keep their bitmap.
--
-- Both columns are dropped guarded so fresh schemas (which never had them)
-- and already-migrated databases are both no-ops.

SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'grade_level') > 0,
  'ALTER TABLE problems DROP COLUMN grade_level',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'settings' AND COLUMN_NAME = 'grade_level') > 0,
  'ALTER TABLE settings DROP COLUMN grade_level',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

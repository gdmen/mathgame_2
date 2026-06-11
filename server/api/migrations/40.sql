-- #225 PR3: grade_level is fully removed. The bitmap now fully describes
-- "what shape of problem is OK for this kid" and target_difficulty is the
-- adaptive lever within that envelope. No settings translation: existing
-- users keep their bitmap (the UI presets are display sugar only).
--
-- Selection stopped reading problems.grade_level in this release and the
-- model layer stopped reading settings.grade_level at the same time.
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

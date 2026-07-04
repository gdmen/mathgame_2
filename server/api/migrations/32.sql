UPDATE settings SET target_difficulty = IF(grade_level > 0, grade_level * 2 + 4, 20) WHERE target_difficulty > IF(grade_level > 0, grade_level * 2 + 4, 20);
UPDATE topic_stats SET target_difficulty = 20, attempts = 0, correct = 0 WHERE target_difficulty > 20;
-- Guarded on the disabled column still existing. Migration 46 replaced disabled
-- with a status ENUM and dropped the column, so on a fresh bootstrap (whose
-- generated problems table never had disabled) this sweep is a no-op on an empty
-- table rather than an unknown-column error.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'disabled') > 0,
  'UPDATE problems SET disabled = 1 WHERE difficulty > 50 AND disabled = 0',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

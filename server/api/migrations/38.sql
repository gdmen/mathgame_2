-- Track which version of ComputeProblemDifficulty produced each row's
-- stored difficulty value. Empty-string default backfills existing rows.
-- New rows and recomputes stamp the current DifficultyVersion explicitly.
-- Idempotent via INFORMATION_SCHEMA check.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'difficulty_version') = 0,
  "ALTER TABLE problems ADD COLUMN difficulty_version VARCHAR(16) NOT NULL DEFAULT ''",
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

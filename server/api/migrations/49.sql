-- When the row was generated. problems.id is a content hash, not a sequence,
-- so rows otherwise carry no time signal. Appended last to match the field
-- order in models.json, which the generated SELECT * scans positionally.
-- Existing rows get the migration-time default. Their real creation times are
-- unrecoverable. Idempotent via INFORMATION_SCHEMA check.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'created_at') = 0,
  'ALTER TABLE problems ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

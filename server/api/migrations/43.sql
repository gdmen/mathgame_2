-- The canonical symbolic form a word problem computes (e.g. 9999 / 3 / 3),
-- used to score its difficulty. Empty for non-word problems, whose expression
-- is already symbolic. Empty-string default backfills existing rows. New rows
-- stamp it explicitly. Idempotent via INFORMATION_SCHEMA check.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'symbolic_expression') = 0,
  "ALTER TABLE problems ADD COLUMN symbolic_expression VARCHAR(512) NOT NULL DEFAULT '' AFTER explanation",
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

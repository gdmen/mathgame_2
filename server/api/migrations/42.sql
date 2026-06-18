-- Cache for the admin difficulty-calibration report. A single row (id = 1)
-- holds the serialized report and when it was computed. The report is rebuilt
-- on demand from the admin page. Guarded so a fresh schema and a re-run are
-- both no-ops.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'calibration_report') = 0,
  'CREATE TABLE calibration_report (id TINYINT UNSIGNED PRIMARY KEY, report LONGTEXT NOT NULL, computed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

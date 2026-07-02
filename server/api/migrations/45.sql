-- Cache for the admin bitmap × difficulty coverage matrix report. A single row
-- (id = 1) holds the gzipped serialized report and when it was computed. The
-- report live-generates one heuristic_2.0 example per (bitmap, difficulty) cell
-- across the whole valid bitmap space and is rebuilt on demand from the admin
-- page. The blob is gzipped (the raw JSON is ~10MB) and the GET endpoint serves
-- the stored bytes as-is with Content-Encoding gzip. Guarded so a fresh schema
-- and a re-run are both no-ops.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'bitmap_matrix_report') = 0,
  'CREATE TABLE bitmap_matrix_report (id TINYINT UNSIGNED PRIMARY KEY, report LONGBLOB NOT NULL, computed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

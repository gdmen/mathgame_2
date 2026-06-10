-- #225 PR2: selection moved from permutation-IN(...) to the bitwise-subset
-- clause (problem_type_bitmap & ~enabled) = 0. The replacement index is
-- (disabled, difficulty, problem_type_bitmap): equality seek on disabled +
-- range scan on difficulty, with problem_type_bitmap IN THE INDEX so the
-- subset filter is evaluated from index pages (covering - no row lookups).
--
-- Measured on a 320K-row synthetic copy (39_explain_analyze.md): a covering
-- index answers the selection query in ~20ms vs ~130ms for a plain
-- (disabled, difficulty) index that must fetch rows to test the bitmap -
-- the trailing bitmap column is what earns the index its keep. grade_level
-- left the query entirely (the column is dropped in migration 40), so the
-- old 4-column composite is replaced rather than kept.

-- 1. Create the new covering index first (create-then-drop keeps a usable
-- index on the table at every point during the migration).
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_disabled_diff_bitmap') = 0,
  'CREATE INDEX idx_problems_disabled_diff_bitmap ON problems (disabled, difficulty, problem_type_bitmap)',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2. Drop the old 4-column composite (carried grade_level, now unread).
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_disabled_diff_grade_bitmap') > 0,
  'DROP INDEX idx_problems_disabled_diff_grade_bitmap ON problems',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

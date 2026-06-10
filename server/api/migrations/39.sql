-- #225 PR2: selection moved from permutation-IN(...) to the bitwise-subset
-- clause (problem_type_bitmap & ~enabled) = 0. A bitwise expression can't be
-- index-seeked, so the 4-column composite from migration 37 no longer earns
-- its keep: the planner now narrows on (disabled, difficulty) - equality
-- seek + range scan - and evaluates the bitmap subset test on the surviving
-- rows. grade_level left the query entirely (the column is dropped in
-- migration 40). See migrations/39_explain_analyze.md for the EXPLAIN
-- ANALYZE artifact on a prod-sized copy.

-- 1. Create the narrowed index first (create-then-drop keeps a usable index
-- on the table at every point during the migration).
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_disabled_difficulty') = 0,
  'CREATE INDEX idx_problems_disabled_difficulty ON problems (disabled, difficulty)',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2. Drop the old 4-column composite.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_disabled_diff_grade_bitmap') > 0,
  'DROP INDEX idx_problems_disabled_diff_grade_bitmap ON problems',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

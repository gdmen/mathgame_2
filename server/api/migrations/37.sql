-- 1. Drop the redundant id non-PRIMARY index on problems. It was
-- created by `BIGINT UNSIGNED PRIMARY KEY UNIQUE` in the original
-- model definition. PRIMARY KEY already implies uniqueness, so the
-- trailing UNIQUE built a second redundant index. models.json has
-- been corrected so fresh schemas no longer create it. This block
-- cleans up existing databases.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'id') > 0,
  'DROP INDEX id ON problems',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2. Create the composite index that lets `getSatisfyingProblemIds`
-- and `getSatisfyingProblemIdsForTopic` narrow with TWO index columns
-- (equality seek on disabled + range scan on difficulty) before
-- applying the remaining filters via Index Condition Pushdown.
-- Leading with disabled=0 lands in the 60% subtree, then the ±30%
-- difficulty window further cuts to ~3.6% of that. Without this
-- index, every Stage 1/2 hit scans the entire problems table
-- (observed at 3.5s in prod with 316K rows).
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_disabled_diff_grade_bitmap') = 0,
  'CREATE INDEX idx_problems_disabled_diff_grade_bitmap ON problems (disabled, difficulty, grade_level, problem_type_bitmap)',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

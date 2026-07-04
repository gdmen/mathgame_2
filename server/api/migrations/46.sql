-- Replace the problems.disabled boolean with a status ENUM that separates a
-- problem's provenance: 'active' (served + counted), 'deprecated' (valid when
-- shown, no longer served), 'reported' (an unvalidated bad-problem claim), and
-- 'incorrect' (admin-confirmed wrong, starts empty). See docs/schema.md.
--
-- Every statement is INFORMATION_SCHEMA-guarded so a fresh schema (whose
-- generated CREATE TABLE already carries status and never had disabled) and an
-- already-migrated database both converge, and a re-run after a mid-failure
-- column drop no-ops instead of erroring on a WHERE disabled reference.

-- 1. Add status if absent. On a fresh DB the generated CREATE already has it.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'status') = 0,
  'ALTER TABLE problems ADD COLUMN status ENUM(''active'',''deprecated'',''reported'',''incorrect'') NOT NULL DEFAULT ''active'' AFTER difficulty',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2. Provenance-aware backfill of the old disabled rows (guarded on the column
-- still existing). Rows flagged by a bad_problem_* event are unvalidated claims
-- and become 'reported'. The event's problem_id is stored in TWO historical
-- value formats -- a bare integer from the original client and a JSON object
-- since the parent-report feature added an explanation field -- so the join
-- must handle both, else it silently misses half the evidence. The JSON branch
-- is JSON_VALID-guarded so a stray empty/malformed value yields NULL (matches no
-- id, falls through to deprecated below) rather than raising ER_INVALID_JSON_TEXT
-- and wedging this one-shot migration mid-apply.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'disabled') > 0,
  'UPDATE problems p SET p.status = ''reported'' WHERE p.disabled = 1 AND EXISTS (SELECT 1 FROM events e WHERE e.event_type IN (''bad_problem_system'',''bad_problem_user'') AND (CASE WHEN e.value REGEXP ''^[0-9]+$'' THEN CAST(e.value AS UNSIGNED) WHEN JSON_VALID(e.value) THEN CAST(JSON_EXTRACT(e.value, ''$.problem_id'') AS UNSIGNED) ELSE NULL END) = p.id)',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 3. Remaining disabled rows had no bad_problem event -- they were pulled by the
-- migration-32 difficulty sweep, a serving decision, not a claim they are wrong.
-- That is the 'deprecated' definition. Runs after step 2 so an event-flagged row
-- stays 'reported'.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'disabled') > 0,
  'UPDATE problems SET status = ''deprecated'' WHERE disabled = 1 AND status = ''active''',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 4. Retire the pre-current generator versions: valid when generated but no
-- longer served. Touches only rows still 'active', so an event-flagged or
-- swept old-gen row keeps its disabled-provenance status from steps 2-3.
UPDATE problems SET status = 'deprecated'
  WHERE generator IN ('heuristic_0.0', 'llm_0.0', 'llm_0.1', 'llm_0.2') AND status = 'active';

-- 5. Create the status serving index first (create-then-drop keeps a usable
-- index on the selection path at every point). Mirrors the disabled index it
-- replaces: equality seek on status + range on difficulty + covering bitmap.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_status_diff_bitmap') = 0,
  'CREATE INDEX idx_problems_status_diff_bitmap ON problems (status, difficulty, problem_type_bitmap)',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 6. Drop the old disabled serving index.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND INDEX_NAME = 'idx_problems_disabled_diff_bitmap') > 0,
  'DROP INDEX idx_problems_disabled_diff_bitmap ON problems',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 7. Drop the disabled column, now that status carries its meaning. Forward-only
-- (rollback is a new higher-numbered migration).
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'problems' AND COLUMN_NAME = 'disabled') > 0,
  'ALTER TABLE problems DROP COLUMN disabled',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

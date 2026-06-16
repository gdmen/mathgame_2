-- Add a server-side authorization role to users. Default 'student'; the single
-- operator is promoted manually (UPDATE users SET role='admin' WHERE auth0_id=...).
-- Guarded so a fresh schema (whose generated CREATE TABLE already includes role)
-- and an already-migrated database are both no-ops.
SET @sql = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'role') = 0,
  'ALTER TABLE users ADD COLUMN role VARCHAR(16) NOT NULL DEFAULT ''student''',
  'SELECT 1'
));
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Backfill you_tube_id from url for existing videos (youtube.com/watch?v=ID or youtu.be/ID)
UPDATE videos
SET you_tube_id = CASE
  WHEN url LIKE '%youtu.be/%' THEN LEFT(SUBSTRING_INDEX(url, '/', -1), 11)
  WHEN url LIKE '%v=%' THEN LEFT(SUBSTRING_INDEX(SUBSTRING_INDEX(url, 'v=', -1), '&', 1), 11)
  ELSE you_tube_id
END
WHERE you_tube_id IS NULL
  AND (url LIKE '%youtu.be/%' OR url LIKE '%v=%');

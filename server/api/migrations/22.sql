-- Consolidate video rows that refer to the same YouTube video (same canonical key from url: v=ID or youtu.be/ID). Pick one winner per key (prefer not disabled), then drop user_id.
-- Step 0: Canonical key per video (YouTube video ID from url, or full url if not parseable).
CREATE TEMPORARY TABLE video_canonical (video_id BIGINT UNSIGNED NOT NULL, canonical_key VARCHAR(256) NOT NULL COLLATE utf8mb4_unicode_ci);

INSERT INTO video_canonical (video_id, canonical_key)
SELECT id,
  CASE
    WHEN url LIKE '%youtu.be/%' THEN LEFT(SUBSTRING_INDEX(url, '/', -1), 11)
    WHEN url LIKE '%v=%' THEN LEFT(SUBSTRING_INDEX(SUBSTRING_INDEX(url, 'v=', -1), '&', 1), 11)
    ELSE url
  END
FROM videos;

-- Step 1: (winner_id, canonical_key) for each canonical_key that has more than one video. Winner = not disabled if any, else any. Use dup_keys so we don't reference video_canonical twice (MySQL can't reopen temp table).
CREATE TEMPORARY TABLE dup_keys (canonical_key VARCHAR(256) NOT NULL COLLATE utf8mb4_unicode_ci);

INSERT INTO dup_keys SELECT canonical_key FROM video_canonical GROUP BY canonical_key HAVING COUNT(*) > 1;

CREATE TEMPORARY TABLE url_winner (winner_id BIGINT UNSIGNED NOT NULL, canonical_key VARCHAR(256) NOT NULL COLLATE utf8mb4_unicode_ci);

INSERT INTO url_winner (winner_id, canonical_key)
SELECT (SELECT v2.id FROM videos v2 INNER JOIN video_canonical vc2 ON vc2.video_id = v2.id WHERE vc2.canonical_key = d.canonical_key ORDER BY v2.disabled ASC, v2.id ASC LIMIT 1), d.canonical_key
FROM dup_keys d;

DROP TEMPORARY TABLE IF EXISTS dup_keys;

-- Step 2: (loser_id, winner_id) for each video we will remove.
CREATE TEMPORARY TABLE losers (loser_id BIGINT UNSIGNED NOT NULL, winner_id BIGINT UNSIGNED NOT NULL);

INSERT INTO losers (loser_id, winner_id)
SELECT vc.video_id, w.winner_id FROM video_canonical vc INNER JOIN url_winner w ON vc.canonical_key = w.canonical_key WHERE vc.video_id != w.winner_id;

DROP TEMPORARY TABLE IF EXISTS video_canonical;

-- Step 3: Point done_watching_video events at the winner.
UPDATE events e INNER JOIN losers l ON CAST(e.value AS UNSIGNED) = l.loser_id SET e.value = CAST(l.winner_id AS CHAR) WHERE e.event_type = 'done_watching_video';

-- Step 3b: Backfill user_has_video from videos.user_id so Step 4 can remap loser->winner for user-owned videos.
INSERT IGNORE INTO user_has_video (user_id, video_id) SELECT user_id, id FROM videos WHERE user_id > 0;

-- Step 4: Ensure every user who had a loser has user_has_video for the winner, then remove loser rows.
INSERT IGNORE INTO user_has_video (user_id, video_id) SELECT DISTINCT uhv.user_id, l.winner_id FROM user_has_video uhv INNER JOIN losers l ON uhv.video_id = l.loser_id;

DELETE uhv FROM user_has_video uhv INNER JOIN losers l ON uhv.video_id = l.loser_id;

-- Step 5: Gamestates: point at winner.
UPDATE gamestates g INNER JOIN losers l ON g.video_id = l.loser_id SET g.video_id = l.winner_id;

-- Step 6: Playlist_video: ensure each playlist has the winner, then remove loser rows.
INSERT IGNORE INTO playlist_video (playlist_id, video_id) SELECT DISTINCT pv.playlist_id, l.winner_id FROM playlist_video pv INNER JOIN losers l ON pv.video_id = l.loser_id;

DELETE pv FROM playlist_video pv INNER JOIN losers l ON pv.video_id = l.loser_id;

-- Step 7: Remove losing video rows.
DELETE v FROM videos v INNER JOIN losers l ON v.id = l.loser_id;

DROP TEMPORARY TABLE IF EXISTS losers;
DROP TEMPORARY TABLE IF EXISTS url_winner;

-- Step 7b: Ensure every video with a valid user_id has a row in user_has_video.
INSERT IGNORE INTO user_has_video (user_id, video_id) SELECT user_id, id FROM videos WHERE user_id > 0;

-- Step 8: Remove user_id from videos.
ALTER TABLE videos DROP COLUMN user_id;

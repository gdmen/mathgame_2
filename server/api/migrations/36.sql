-- Bounded per-user cache of the most-recently-shown problems. Read by
-- process_events.go (prevIds hard-exclude) and select_lru.go (recency-bias
-- staleness sort) instead of scanning the big events table. Trimmed
-- asynchronously to recentlyShownProblemsTrimSize rows per user.
CREATE TABLE IF NOT EXISTS recently_shown_problems (
    user_id      INT UNSIGNED NOT NULL,
    problem_id   INT UNSIGNED NOT NULL,
    shown_at     TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, problem_id),
    KEY idx_recently_shown_problems_user_time (user_id, shown_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

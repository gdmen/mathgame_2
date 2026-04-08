CREATE TABLE IF NOT EXISTS topic_stats (
    user_id BIGINT UNSIGNED NOT NULL,
    problem_type BIGINT UNSIGNED NOT NULL,
    attempts INT UNSIGNED NOT NULL DEFAULT 0,
    correct INT UNSIGNED NOT NULL DEFAULT 0,
    target_difficulty DOUBLE NOT NULL DEFAULT 3,
    PRIMARY KEY (user_id, problem_type)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE statistics_cache_meta (
	user_id BIGINT UNSIGNED PRIMARY KEY,
	last_event_id BIGINT UNSIGNED NOT NULL DEFAULT 0
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE statistics_totals (
	user_id BIGINT UNSIGNED PRIMARY KEY,
	total_problems_solved BIGINT NOT NULL DEFAULT 0,
	total_work_minutes BIGINT NOT NULL DEFAULT 0,
	total_video_minutes BIGINT NOT NULL DEFAULT 0
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE statistics_monthly (
	user_id BIGINT UNSIGNED NOT NULL,
	month CHAR(7) NOT NULL,
	total_problems_solved BIGINT NOT NULL DEFAULT 0,
	total_work_minutes BIGINT NOT NULL DEFAULT 0,
	total_video_minutes BIGINT NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, month)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE statistics_hardest_aggregates (
	user_id BIGINT UNSIGNED NOT NULL,
	problem_id INT UNSIGNED NOT NULL,
	total_time_ms BIGINT NOT NULL DEFAULT 0,
	total_attempts INT NOT NULL DEFAULT 0,
	solve_count INT NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, problem_id)
) DEFAULT CHARSET=utf8mb4;

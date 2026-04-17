UPDATE settings SET target_difficulty = grade_level * 2 WHERE grade_level > 0;
UPDATE topic_stats ts INNER JOIN settings s ON ts.user_id = s.user_id SET ts.target_difficulty = s.grade_level * 2, ts.attempts = 0, ts.correct = 0 WHERE s.grade_level > 0;

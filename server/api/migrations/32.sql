UPDATE settings SET target_difficulty = IF(grade_level > 0, grade_level * 2 + 4, 20) WHERE target_difficulty > IF(grade_level > 0, grade_level * 2 + 4, 20);
UPDATE topic_stats SET target_difficulty = 20, attempts = 0, correct = 0 WHERE target_difficulty > 20;
UPDATE problems SET disabled = 1 WHERE difficulty > 50 AND disabled = 0;

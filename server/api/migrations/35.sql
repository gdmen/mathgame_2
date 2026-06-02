UPDATE problems SET grade_level = GREATEST(1, LEAST(8, CAST(FLOOR((difficulty - 4) / 2) AS SIGNED))) WHERE grade_level = 0 AND generator != 'heuristic_0.0' AND disabled = 0;

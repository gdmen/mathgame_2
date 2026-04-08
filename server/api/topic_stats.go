package api

import (
	"github.com/golang/glog"
)

// TopicStat holds per-topic accuracy and difficulty for a single user+topic.
type TopicStat struct {
	UserID           uint32  `json:"user_id"`
	ProblemType      uint64  `json:"problem_type"`
	Attempts         uint32  `json:"attempts"`
	Correct          uint32  `json:"correct"`
	TargetDifficulty float64 `json:"target_difficulty"`
}

func (ts TopicStat) Accuracy() float64 {
	if ts.Attempts == 0 {
		return 0
	}
	return float64(ts.Correct) / float64(ts.Attempts)
}

// getTopicStats returns all topic_stats rows for a user, keyed by ProblemType.
func (a *Api) getTopicStats(userID uint32) (map[uint64]*TopicStat, error) {
	rows, err := a.DB.Query(
		`SELECT user_id, problem_type, attempts, correct, target_difficulty FROM topic_stats WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := make(map[uint64]*TopicStat)
	for rows.Next() {
		ts := &TopicStat{}
		if err := rows.Scan(&ts.UserID, &ts.ProblemType, &ts.Attempts, &ts.Correct, &ts.TargetDifficulty); err != nil {
			return nil, err
		}
		stats[ts.ProblemType] = ts
	}
	return stats, rows.Err()
}

// getOrDefaultTopicStat returns the stat for a topic, or a default with the given base difficulty.
func getOrDefaultTopicStat(stats map[uint64]*TopicStat, userID uint32, problemType uint64, baseDifficulty float64) *TopicStat {
	if ts, ok := stats[problemType]; ok {
		return ts
	}
	return &TopicStat{
		UserID:           userID,
		ProblemType:      problemType,
		Attempts:         0,
		Correct:          0,
		TargetDifficulty: baseDifficulty,
	}
}

// recordTopicAttempt increments attempts (and correct if isCorrect) for each
// individual problem type bit set in the problem's bitmap.
func (a *Api) recordTopicAttempt(logPrefix string, userID uint32, problemTypeBitmap uint64, isCorrect bool, baseDifficulty float64) {
	for i := 0; i < 64; i++ {
		pt := uint64(1 << i)
		if (problemTypeBitmap & pt) == 0 {
			continue
		}
		correctDelta := 0
		if isCorrect {
			correctDelta = 1
		}
		_, err := a.DB.Exec(`
			INSERT INTO topic_stats (user_id, problem_type, attempts, correct, target_difficulty)
			VALUES (?, ?, 1, ?, ?)
			ON DUPLICATE KEY UPDATE
				attempts = attempts + 1,
				correct = correct + ?`,
			userID, pt, correctDelta, baseDifficulty, correctDelta,
		)
		if err != nil {
			glog.Errorf("%s recordTopicAttempt: %v", logPrefix, err)
		}
	}
}

// adjustTopicDifficulty adjusts target_difficulty for topics with enough data.
// Topics with <minAttempts are left unchanged. Uses the same adjustment logic
// as the global difficulty adjuster in process_events.go.
func (a *Api) adjustTopicDifficulty(logPrefix string, userID uint32, stats map[uint64]*TopicStat) {
	const minAttempts = 10
	const diffIncrease = 0.05
	const minDiff = 3.0

	for pt, ts := range stats {
		if ts.Attempts < uint32(minAttempts) {
			continue
		}
		acc := ts.Accuracy()
		newDiff := ts.TargetDifficulty

		if acc > 0.80 {
			// Doing well: make harder
			bump := diffIncrease * newDiff
			if bump < 1 {
				bump = 1
			}
			newDiff += bump
		} else if acc < 0.50 {
			// Struggling: make easier
			drop := diffIncrease * newDiff
			if drop < 1 {
				drop = 1
			}
			newDiff -= drop
			if newDiff < minDiff {
				newDiff = minDiff
			}
		}

		if newDiff != ts.TargetDifficulty {
			glog.Infof("%s adjustTopicDifficulty: user=%d topic=%d accuracy=%.2f difficulty %.2f -> %.2f",
				logPrefix, userID, pt, acc, ts.TargetDifficulty, newDiff)
			_, err := a.DB.Exec(
				`UPDATE topic_stats SET target_difficulty = ? WHERE user_id = ? AND problem_type = ?`,
				newDiff, userID, pt,
			)
			if err != nil {
				glog.Errorf("%s adjustTopicDifficulty update: %v", logPrefix, err)
			}
		}

		// Reset counters after adjustment so the next window starts fresh
		_, err := a.DB.Exec(
			`UPDATE topic_stats SET attempts = 0, correct = 0 WHERE user_id = ? AND problem_type = ?`,
			userID, pt,
		)
		if err != nil {
			glog.Errorf("%s adjustTopicDifficulty reset: %v", logPrefix, err)
		}
	}
}

// chooseWeightedTopic picks a problem type to focus on. Weak topics (accuracy < 60%
// with 10+ attempts) are weighted 2x. Returns a single ProblemType value and its
// target difficulty. If no stats exist, returns 0 (caller should use default behavior).
func chooseWeightedTopic(stats map[uint64]*TopicStat, enabledBitmap uint64, baseDifficulty float64, rng func(int) int) (uint64, float64) {
	type candidate struct {
		problemType uint64
		difficulty  float64
		weight      int
	}

	var candidates []candidate
	for i := 0; i < 64; i++ {
		pt := uint64(1 << i)
		if (enabledBitmap & pt) == 0 {
			continue
		}
		diff := baseDifficulty
		weight := 1
		if ts, ok := stats[pt]; ok {
			diff = ts.TargetDifficulty
			if ts.Attempts >= 10 && ts.Accuracy() < 0.60 {
				weight = 2 // Weak topic: 2x weight
			}
		}
		candidates = append(candidates, candidate{pt, diff, weight})
	}

	if len(candidates) == 0 {
		return 0, baseDifficulty
	}

	totalWeight := 0
	for _, c := range candidates {
		totalWeight += c.weight
	}
	pick := rng(totalWeight)
	cumulative := 0
	for _, c := range candidates {
		cumulative += c.weight
		if pick < cumulative {
			return c.problemType, c.difficulty
		}
	}
	// Shouldn't reach here, but return last candidate
	last := candidates[len(candidates)-1]
	return last.problemType, last.difficulty
}

// topicStatsForUser initializes topic_stats rows for any enabled topics that
// don't have rows yet. Called when creating a new user or updating settings.
func (a *Api) initTopicStats(userID uint32, enabledBitmap uint64, baseDifficulty float64) error {
	for i := 0; i < 64; i++ {
		pt := uint64(1 << i)
		if (enabledBitmap & pt) == 0 {
			continue
		}
		_, err := a.DB.Exec(`
			INSERT IGNORE INTO topic_stats (user_id, problem_type, attempts, correct, target_difficulty)
			VALUES (?, ?, 0, 0, ?)`,
			userID, pt, baseDifficulty,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// getEffectiveDifficulty returns the difficulty to use for problem selection.
// Uses per-topic difficulty if available with enough data, otherwise falls back
// to the global settings difficulty.
func getEffectiveDifficulty(stats map[uint64]*TopicStat, targetTopic uint64, baseDifficulty float64) float64 {
	if ts, ok := stats[targetTopic]; ok {
		return ts.TargetDifficulty
	}
	return baseDifficulty
}

// topicMatchesFilter returns true if the given problem type bitmap contains
// the target topic (or if targetTopic is 0, meaning no filtering).
func topicMatchesFilter(problemTypeBitmap uint64, targetTopic uint64) bool {
	if targetTopic == 0 {
		return true
	}
	return (problemTypeBitmap & targetTopic) != 0
}

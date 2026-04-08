package api

import (
	"time"

	"github.com/golang/glog"
)

// Spaced repetition intervals: 1 day -> 3 days -> 7 days -> done (removed from queue)
var spacedRepIntervals = []int{1, 3, 7}

// addToReviewQueue inserts a problem into the review queue on incorrect answer.
// If the problem is already in the queue, resets it to the first interval.
func (a *Api) addToReviewQueue(logPrefix string, userID uint32, problemID uint32) {
	nextReview := time.Now().Add(24 * time.Hour)
	_, err := a.DB.Exec(`
		INSERT INTO review_queue (user_id, problem_id, next_review_at, interval_days)
		VALUES (?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE
			next_review_at = VALUES(next_review_at),
			interval_days = 1`,
		userID, problemID, nextReview,
	)
	if err != nil {
		glog.Errorf("%s addToReviewQueue: %v", logPrefix, err)
	}
}

// advanceReviewQueue advances the interval for a correctly-answered review problem.
// If the problem has completed all intervals, it's removed from the queue.
func (a *Api) advanceReviewQueue(logPrefix string, userID uint32, problemID uint32) {
	var currentInterval int
	err := a.DB.QueryRow(
		`SELECT interval_days FROM review_queue WHERE user_id = ? AND problem_id = ?`,
		userID, problemID,
	).Scan(&currentInterval)
	if err != nil {
		// Not in review queue, nothing to do
		return
	}

	// Find the next interval
	nextIntervalIdx := -1
	for i, interval := range spacedRepIntervals {
		if interval == currentInterval {
			nextIntervalIdx = i + 1
			break
		}
	}

	if nextIntervalIdx < 0 || nextIntervalIdx >= len(spacedRepIntervals) {
		// Completed all intervals, remove from queue
		_, err = a.DB.Exec(
			`DELETE FROM review_queue WHERE user_id = ? AND problem_id = ?`,
			userID, problemID,
		)
		if err != nil {
			glog.Errorf("%s advanceReviewQueue delete: %v", logPrefix, err)
		}
		glog.Infof("%s spaced rep: user=%d problem=%d completed all intervals, removed from queue", logPrefix, userID, problemID)
		return
	}

	nextInterval := spacedRepIntervals[nextIntervalIdx]
	nextReview := time.Now().Add(time.Duration(nextInterval) * 24 * time.Hour)
	_, err = a.DB.Exec(
		`UPDATE review_queue SET interval_days = ?, next_review_at = ? WHERE user_id = ? AND problem_id = ?`,
		nextInterval, nextReview, userID, problemID,
	)
	if err != nil {
		glog.Errorf("%s advanceReviewQueue update: %v", logPrefix, err)
	}
	glog.Infof("%s spaced rep: user=%d problem=%d advanced to %d-day interval", logPrefix, userID, problemID, nextInterval)
}

// getDueReviewProblem returns a problem ID from the review queue that is due for
// review (next_review_at <= now). Returns 0 if none are due.
func (a *Api) getDueReviewProblem(logPrefix string, userID uint32) uint32 {
	var problemID uint32
	err := a.DB.QueryRow(`
		SELECT problem_id FROM review_queue
		WHERE user_id = ? AND next_review_at <= NOW()
		ORDER BY next_review_at ASC
		LIMIT 1`,
		userID,
	).Scan(&problemID)
	if err != nil {
		// No due reviews, that's fine
		return 0
	}
	glog.Infof("%s spaced rep: user=%d has due review problem=%d", logPrefix, userID, problemID)
	return problemID
}

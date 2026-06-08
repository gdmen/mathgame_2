package api

import (
	"fmt"
	"strings"
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

// getDueReviewProblem returns a problem ID from the review queue that is
// due for review AND still matches the user's current settings:
//   - problem_type_bitmap is one of the permutations of currently-enabled
//     topics (so a previously-failed problem with a now-disabled topic bit
//     stops surfacing as a review),
//   - difficulty <= target_difficulty + problemSelectionEpsilon — no
//     lower bound, since a now-easy review is still a meaningful retest,
//   - grade_level > 0 (skip legacy backfill sentinel rows),
//   - not disabled.
//
// Unlike the Stage 1/2 pool selection, this does NOT apply the
// WORD-only-when-WORD-enabled special case: spaced rep targets a specific
// problem the user previously got wrong, regardless of WORD setting.
//
// Returns 0 if no due reviews match. Caller (selectProblem) then falls
// through to the normal topic-weighted / default selection path.
func (a *Api) getDueReviewProblem(logPrefix string, settings *Settings) uint32 {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	if len(permutations) == 0 {
		return 0
	}
	// Same Sprintf+Replace interpolation pattern used by getSatisfyingProblemIds
	// in generate_problems.go — keeps all three SQL sites consistent.
	permsStr := strings.Replace(strings.Trim(fmt.Sprint(permutations), "[]"), " ", ",", -1)
	diffUpperBound := settings.TargetDifficulty + problemSelectionEpsilon

	// Earliest-due review problem for this user, gated by current settings.
	// JOINs review_queue (small, per-user) against the indexed problems
	// table by primary key; the filters drop rows that no longer fit the
	// user's current topic/difficulty/grade.
	sql := fmt.Sprintf(`
		SELECT rq.problem_id
		FROM review_queue rq
		JOIN problems p ON p.id = rq.problem_id
		WHERE rq.user_id = ?
		  AND rq.next_review_at <= NOW()
		  AND p.disabled = 0
		  AND p.grade_level > 0
		  AND p.problem_type_bitmap IN (%s)
		  AND p.difficulty <= ?
		ORDER BY rq.next_review_at ASC
		LIMIT 1`, permsStr)

	var problemID uint32
	if err := a.DB.QueryRow(sql, settings.UserId, diffUpperBound).Scan(&problemID); err != nil {
		// No matching due reviews; fall through to the rest of selectProblem.
		return 0
	}
	glog.Infof("%s spaced rep: user=%d has due review problem=%d", logPrefix, settings.UserId, problemID)
	return problemID
}

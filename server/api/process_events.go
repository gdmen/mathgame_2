// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

// parseBadProblemID returns the problem_id from a BAD_PROBLEM_* event's
// JSON value, or 0 if the value is empty / not JSON / missing the field.
func parseBadProblemID(rawValue string) uint32 {
	if rawValue == "" {
		return 0
	}
	var v struct {
		ProblemID uint32 `json:"problem_id"`
	}
	if err := json.Unmarshal([]byte(rawValue), &v); err != nil {
		return 0
	}
	return v.ProblemID
}

const (
	maxTarget = 20
)

// recentProblemHistorySize and the other recency-related sizes live in
// generate_problems.go alongside the rest of the selection-funnel constants.

// processRecordOnlyEvents persists events that do not mutate gamestate or settings.
// Use this for LOGGED_IN, WORKING_ON_PROBLEM, WATCHING_VIDEO, SET_TARGET_WORK_PERCENTAGE.
func (a *Api) processRecordOnlyEvents(logPrefix string, c *gin.Context, events []*Event) error {
	user := GetUserFromContext(c)
	if err := a.createEventsBatch(user.Id, events); err != nil {
		glog.Errorf("%s createEventsBatch: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Couldn't add events to database"))
		return err
	}
	return nil
}

// processEvents dispatches to a simple path for record-only events, or the full
// gamestate path when any event mutates gamestate/settings.
func (a *Api) processEvents(logPrefix string, c *gin.Context, events []*Event, writeCtx bool) error {
	if len(events) == 0 {
		return nil
	}
	allRecordOnly := true
	for _, e := range events {
		if !isRecordOnlyEvent(e.EventType) {
			allRecordOnly = false
			break
		}
	}
	// Use simple path only when all events are record-only AND we don't need play data
	if allRecordOnly && !writeCtx {
		return a.processRecordOnlyEvents(logPrefix, c, events)
	}

	// Get User
	user := GetUserFromContext(c)

	// Get Gamestate + Settings in a single round-trip via JOIN. Both rows
	// are keyed by user_id and we always need both in this code path.
	gamestate, settings, err := a.loadGamestateAndSettings(user.Id)
	if err != nil {
		if err == sql.ErrNoRows {
			glog.Errorf("%s gamestate or settings missing for user=%d", logPrefix, user.Id)
			c.JSON(http.StatusNotFound, common.GetError("gamestate or settings not found"))
			return err
		}
		glog.Errorf("%s loadGamestateAndSettings: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("could not load gamestate/settings"))
		return err
	}
	glog.Infof("%s Gamestate: %v", logPrefix, gamestate)
	glog.Infof("%s Settings: %v", logPrefix, settings)

	for _, event := range events {
		err = a.processEvent(logPrefix, c, event, writeCtx, user, gamestate, settings)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *Api) processEvent(logPrefix string, c *gin.Context, event *Event, writeCtx bool, user *User, gamestate *Gamestate, settings *Settings) error {

	changed_gamestate := false
	changed_settings := false
	select_new_problem := false

	// The main event to be processed as well as any side-effect events we add in this function
	events := []*Event{event}

	var changeGamestateTarget = func(val uint32) {
		gamestate.Target = val
		changed_gamestate = true
		events = append(events, &Event{
			EventType: SET_GAMESTATE_TARGET,
			Value:     strconv.FormatUint(uint64(gamestate.Target), 10),
		})
	}

	var changeTargetDifficulty = func(val float64) {
		settings.TargetDifficulty = val
		changed_settings = true
		events = append(events, &Event{
			EventType: SET_TARGET_DIFFICULTY,
			Value:     strconv.FormatFloat(val, 'E', -1, 64),
		})
	}

	if event.EventType == LOGGED_IN {
		// no-op
	} else if event.EventType == SET_TARGET_DIFFICULTY {
		val, parseErr := strconv.ParseFloat(event.Value, 64)
		if parseErr != nil || val < 3 || val > 50 {
			msg := fmt.Sprintf("Invalid target_difficulty: %s (must be 3-50)", event.Value)
			glog.Errorf("%s %s", logPrefix, msg)
			c.JSON(http.StatusBadRequest, msg)
			return errors.New(msg)
		}
		select_new_problem = true
	} else if event.EventType == SET_TARGET_WORK_PERCENTAGE {
		val, parseErr := strconv.ParseUint(event.Value, 10, 8)
		if parseErr != nil || val < 1 || val > 100 {
			msg := fmt.Sprintf("Invalid target_work_percentage: %s (must be 1-100)", event.Value)
			glog.Errorf("%s %s", logPrefix, msg)
			c.JSON(http.StatusBadRequest, msg)
			return errors.New(msg)
		}
	} else if event.EventType == SET_PROBLEM_TYPE_BITMAP {
		val, parseErr := strconv.ParseUint(event.Value, 10, 64)
		if parseErr != nil || val == 0 || val > 255 {
			msg := fmt.Sprintf("Invalid problem_type_bitmap: %s (must be 1-255)", event.Value)
			glog.Errorf("%s %s", logPrefix, msg)
			c.JSON(http.StatusBadRequest, msg)
			return errors.New(msg)
		}
		select_new_problem = true
		a.generateProblemsBackground(logPrefix, settings)
	} else if event.EventType == SET_GAMESTATE_TARGET {
		val, parseErr := strconv.ParseUint(event.Value, 10, 32)
		if parseErr != nil || val < 5 || val > 20 {
			msg := fmt.Sprintf("Invalid gamestate_target: %s (must be 5-20)", event.Value)
			glog.Errorf("%s %s", logPrefix, msg)
			c.JSON(http.StatusBadRequest, msg)
			return errors.New(msg)
		}
	} else if event.EventType == SELECTED_PROBLEM {
		if event.Value != "" {
			val, parseErr := strconv.ParseUint(event.Value, 10, 32)
			if parseErr != nil || val == 0 {
				msg := fmt.Sprintf("Invalid problem_id: %s", event.Value)
				glog.Errorf("%s %s", logPrefix, msg)
				c.JSON(http.StatusBadRequest, msg)
				return errors.New(msg)
			}
		}
	} else if event.EventType == WORKING_ON_PROBLEM {
		val, parseErr := strconv.ParseInt(event.Value, 10, 64)
		if parseErr != nil || val < 0 || val > 3600000 {
			msg := fmt.Sprintf("Invalid working_on_problem duration: %s (must be 0-3600000ms)", event.Value)
			glog.Errorf("%s %s", logPrefix, msg)
			c.JSON(http.StatusBadRequest, msg)
			return errors.New(msg)
		}
	} else if event.EventType == ANSWERED_PROBLEM {
		// Get Problem
		problem, status, msg, err := a.problemManager.Get(gamestate.ProblemId)
		if HandleMngrResp(logPrefix, c, status, msg, err, problem) != nil {
			return err
		}
		if !AnswersEquivalent(event.Value, problem.Answer) {
			msg := fmt.Sprintf("Incorrect answer: {%s}, expected: {%s}", event.Value, problem.Answer)
			glog.Infof("%s %s", logPrefix, msg)
			// Track incorrect attempt per topic
			a.recordTopicAttempt(logPrefix, user.Id, problem.ProblemTypeBitmap, false, settings.TargetDifficulty)
			// Add to spaced repetition review queue
			a.addToReviewQueue(logPrefix, user.Id, gamestate.ProblemId)
		} else { // Answer was correct
			events = append(events, &Event{
				EventType: SOLVED_PROBLEM,
				Value:     strconv.FormatUint(uint64(gamestate.ProblemId), 10),
			})
			// Track correct attempt per topic
			a.recordTopicAttempt(logPrefix, user.Id, problem.ProblemTypeBitmap, true, settings.TargetDifficulty)
			// Advance spaced repetition if this was a review problem
			a.advanceReviewQueue(logPrefix, user.Id, gamestate.ProblemId)
			// Update counts
			gamestate.Solved += 1
			// Select a new problem
			select_new_problem = true
		}
	} else if event.EventType == ERROR_PLAYING_VIDEO {
		// Get the current video
		video, status, msg, err := a.videoManager.Get(gamestate.VideoId)
		if HandleMngrResp(logPrefix, c, status, msg, err, video) != nil {
			return err
		}
		// Disable the current video
		glog.Infof("%s Disabling video: %v", logPrefix, video)
		video.Disabled = true
		// Save the disabled video
		status, msg, err = a.videoManager.Update(video)
		if HandleMngrResp(logPrefix, c, status, msg, err, video) != nil {
			return err
		}
		// Set a new reward video
		videoId, err := a.selectVideo(logPrefix, c, user.Id, map[uint32]bool{gamestate.VideoId: true})
		if err != nil {
			return err
		}
		gamestate.VideoId = videoId
		changed_gamestate = true
	} else if event.EventType == WATCHING_VIDEO {
		// TODO: validate duration
	} else if event.EventType == DONE_WATCHING_VIDEO {
		// TODO: validate videoID

		// Difficulty adjustment limits
		epsilon := 0.05
		var recentPast int = 900 // seconds aka 15 minutes. This assumes a 1 second event reporting interval.
		var diffIncrease float64 = 0.05
		var minDiff float64 = 3
		var minProbs uint32 = 5
		// Maximum difficulty cap, grade-aware when set. Prevents runaway growth.
		// The LLM interprets difficulty as "age in years" - grade 5 kid is ~10-11,
		// so cap at (gradeLevel * 2 + 4). Default ceiling of 20 when grade unset.
		var maxDiff float64 = 20
		if settings.GradeLevel > 0 {
			maxDiff = float64(settings.GradeLevel)*2 + 4
		}
		// End difficulty adjustment limits

		// Defensive: clamp any previously-runaway difficulty back into range.
		// If TargetDifficulty was set to a wildly high value (from the unbounded
		// adjuster bug), reset it to maxDiff immediately so even if later processing
		// fails, the user gets grade-appropriate problems on their next session.
		if settings.TargetDifficulty > maxDiff {
			glog.Infof("%s TargetDifficulty %.2f exceeds cap %.2f, clamping down",
				logPrefix, settings.TargetDifficulty, maxDiff)
			settings.TargetDifficulty = maxDiff
			if _, dbErr := a.DB.Exec(
				`UPDATE settings SET target_difficulty = ? WHERE user_id = ?`,
				maxDiff, user.Id,
			); dbErr != nil {
				glog.Errorf("%s failed to persist difficulty clamp: %v", logPrefix, dbErr)
			}
			// Also log as an event for audit trail
			events = append(events, &Event{
				EventType: SET_TARGET_DIFFICULTY,
				Value:     strconv.FormatFloat(maxDiff, 'E', -1, 64),
			})
		}

		if gamestate.Solved < gamestate.Target {
			msg := fmt.Sprintf("Done watching video, but there's an inconsistency in problems solved: %v < %v", gamestate.Solved, gamestate.Target)
			glog.Errorf("%s %s", logPrefix, msg)
		}
		// Calculate work % for the "recent past" of the user.
		query := `SELECT work/total FROM
                                  (SELECT
                                  SUM(CASE WHEN event_type='working_on_problem' THEN value ELSE 0 END) AS work,
                                  SUM(value) AS total
                                  FROM
                                    (SELECT *
                                    FROM events
                                    WHERE user_id=%d AND event_type IN ('working_on_problem', 'watching_video')
                                    ORDER BY timestamp DESC LIMIT %d) AS X
                                  ) AS Y;`
		value, status, msg, err := a.CustomValueQuery(fmt.Sprintf(query, user.Id, recentPast))
		if HandleMngrResp(logPrefix, c, status, msg, err, value) != nil {
			return err
		}
		workPercentage, err := strconv.ParseFloat(value, 64)
		glog.Infof("%s workPercentage: %v", logPrefix, workPercentage)
		if err != nil {
			return err
		}
		// Adjust work load. Levers are difficulty and target number of problems.
		glog.Infof("%s settings.TargetWorkPercentage: %v", logPrefix, settings.TargetWorkPercentage)
		targetWorkPercentage := float64(settings.TargetWorkPercentage) / 100.0

		glog.Infof("%s starting difficulty & num problems: %v, %v", logPrefix, settings.TargetDifficulty, gamestate.Target)
		// Only do something if we are not already on target
		if math.Abs(targetWorkPercentage-workPercentage) < epsilon {
			glog.Infof("%s difficulty is on target", logPrefix)
		} else if targetWorkPercentage > workPercentage {
			// Make it more difficult
			if gamestate.Target < uint32(maxTarget) {
				changeGamestateTarget(gamestate.Target + 1)
			} else if settings.TargetDifficulty >= maxDiff {
				// Already at difficulty cap - don't bump further, just reset problem target
				glog.Infof("%s difficulty %.2f already at cap %.2f; not increasing further", logPrefix, settings.TargetDifficulty, maxDiff)
				changeGamestateTarget(uint32(math.Max(float64(minProbs), math.Ceil(float64(gamestate.Target)/2))))
			} else {
				changeGamestateTarget(uint32(math.Max(float64(minProbs), math.Ceil(float64(gamestate.Target)/2))))
				newDiff := settings.TargetDifficulty + math.Max(1, diffIncrease*settings.TargetDifficulty)
				if newDiff > maxDiff {
					newDiff = maxDiff
				}
				changeTargetDifficulty(newDiff)
			}
		} else if targetWorkPercentage < workPercentage {
			// Make it easier
			if gamestate.Target > minProbs {
				changeGamestateTarget(uint32(math.Max(float64(minProbs), math.Ceil(float64(gamestate.Target)/2))))
			} else {
				if settings.TargetDifficulty <= minDiff {
					glog.Infof(
						"%s difficulty of %v should not be below %v and we're already at minProbs. Will not make this easier.", logPrefix, settings.TargetDifficulty, minDiff)
					changeTargetDifficulty(minDiff)
				} else {
					changeGamestateTarget(gamestate.Target + 1)
					changeTargetDifficulty(math.Max(minDiff, settings.TargetDifficulty-math.Max(1, diffIncrease*settings.TargetDifficulty)))
				}
			}
		}
		glog.Infof("%s modified difficulty & num problems: %v, %v", logPrefix, settings.TargetDifficulty, gamestate.Target)

		// Adjust per-topic difficulties based on accuracy
		topicStats, tsErr := a.getTopicStats(user.Id)
		if tsErr != nil {
			glog.Errorf("%s getTopicStats: %v", logPrefix, tsErr)
		} else {
			a.adjustTopicDifficulty(logPrefix, user.Id, topicStats)
		}

		// Reset solved progress
		gamestate.Solved = 0
		changed_gamestate = true

		// Set a new reward video
		videoId, err := a.selectVideo(logPrefix, c, user.Id, map[uint32]bool{gamestate.VideoId: true})
		if err != nil {
			return err
		}
		gamestate.VideoId = videoId
		changed_gamestate = true
	} else if event.EventType == BAD_PROBLEM_SYSTEM || event.EventType == BAD_PROBLEM_USER {
		// Disable the reported problem, falling back to gamestate.ProblemId.
		badID := parseBadProblemID(event.Value)
		if badID == 0 {
			badID = gamestate.ProblemId
		}
		problem, status, msg, err := a.problemManager.Get(badID)
		if HandleMngrResp(logPrefix, c, status, msg, err, problem) != nil {
			return err
		}
		glog.Infof("%s Disabling problem: %v", logPrefix, problem)
		problem.Disabled = true
		status, msg, err = a.problemManager.Update(problem)
		if HandleMngrResp(logPrefix, c, status, msg, err, problem) != nil {
			return err
		}
		// Only re-select if the disabled problem is the current one.
		if badID == gamestate.ProblemId {
			select_new_problem = true
		}
	} else {
		msg := fmt.Sprintf("Invalid EventType: %s", event.EventType)
		glog.Errorf("%s %s", logPrefix, msg)
		c.JSON(http.StatusBadRequest, msg)
		return errors.New(msg)
	}

	// Select a new problem
	if select_new_problem {
		// Hard-exclusion list: the recentProblemHistorySize most-recently-shown
		// problem ids for this user, sourced from the bounded
		// recently_shown_problems cache (PK lookup on the (user_id, shown_at)
		// index, sub-millisecond). Fail-tolerant: if the lookup fails the
		// user gets an empty exclusion list (may briefly see a repeat) but
		// the request still serves a problem - matching the upsert path's
		// best-effort posture.
		problemIds := loadRecentProblemIds(logPrefix, a.DB, user.Id)
		problem, err := a.selectProblem(logPrefix, c, settings, &problemIds)
		if err != nil {
			return err
		}
		gamestate.ProblemId = problem.Id
		changed_gamestate = true
	}

	// Write all events to database in a single multi-row INSERT.
	if err := a.createEventsBatch(gamestate.UserId, events); err != nil {
		glog.Errorf("%s createEventsBatch: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Couldn't add events to database"))
		return err
	}
	// After SELECTED_PROBLEM events land, upsert into the
	// recently_shown_problems cache used by the selection funnel.
	// Source-of-truth lives in events; this is a derived cache and a
	// failure here is non-fatal (small drift self-corrects on the next
	// SELECTED_PROBLEM for this user).
	for _, e := range events {
		if e.EventType == SELECTED_PROBLEM {
			recordRecentlyShown(logPrefix, a.DB, e.UserId, e.Value)
		}
	}

	// Write the updated settings
	if changed_settings {
		glog.Infof("%s Settings: %v", logPrefix, settings)
		status, msg, err := a.settingsManager.Update(settings)
		if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
			return err
		}
	}

	// Write the updated gamestate
	if changed_gamestate {
		glog.Infof("%s Gamestate: %v", logPrefix, gamestate)
		status, msg, err := a.gamestateManager.Update(gamestate)
		if HandleMngrResp(logPrefix, c, status, msg, err, gamestate) != nil {
			return err
		}
	}
	if select_new_problem {
		err := a.processEvent(logPrefix, c,
			&Event{
				UserId:    user.Id,
				EventType: SELECTED_PROBLEM,
				Value:     strconv.FormatUint(uint64(gamestate.ProblemId), 10),
			},
			false, user, gamestate, settings,
		)
		if err != nil {
			return err
		}
	}

	// Write the Play data to the response body
	if writeCtx {
		a.helpGetPlayData(logPrefix, c, gamestate)
	}

	return nil
}

// loadRecentProblemIds returns the recentProblemHistorySize most-recently
// shown problem ids for this user from the bounded recently_shown_problems
// cache. Fail-tolerant: on any error, logs a warning and returns an empty
// slice rather than failing the request - a recent-repeat is acceptable
// when the alternative is denying the user a problem at all.
func loadRecentProblemIds(logPrefix string, db *sql.DB, userID uint32) []uint32 {
	out := []uint32{}
	// PK-indexed lookup on (user_id, shown_at).
	rows, err := db.Query(
		`SELECT problem_id FROM recently_shown_problems
		 WHERE user_id = ? ORDER BY shown_at DESC LIMIT ?`,
		userID, recentProblemHistorySize,
	)
	if err != nil {
		glog.Warningf("%s loadRecentProblemIds user=%d query: %v (continuing with empty exclusion list)", logPrefix, userID, err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			glog.Warningf("%s loadRecentProblemIds user=%d scan: %v", logPrefix, userID, err)
			return out
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		glog.Warningf("%s loadRecentProblemIds user=%d iter: %v", logPrefix, userID, err)
		return out
	}
	return out
}

// recordRecentlyShown upserts the (user_id, problem_id, shown_at) row in
// the bounded recently_shown_problems cache. Source of truth lives in the
// events table; this is a derived cache for the selection-funnel hot path.
// Failures are logged but never propagated — small drift here self-corrects
// on the next SELECTED_PROBLEM event for the same user.
func recordRecentlyShown(logPrefix string, db *sql.DB, userID uint32, problemValue string) {
	problemID, err := strconv.ParseUint(problemValue, 10, 32)
	if err != nil || problemID == 0 {
		glog.Warningf("%s recordRecentlyShown: skipping unparseable SELECTED_PROBLEM value %q", logPrefix, problemValue)
		return
	}
	// PK is (user_id, problem_id) so re-shows update shown_at in place
	// instead of piling up. The trim job caps each user at
	// recentlyShownProblemsTrimSize rows.
	_, err = db.Exec(
		`INSERT INTO recently_shown_problems (user_id, problem_id, shown_at)
		 VALUES (?, ?, NOW())
		 ON DUPLICATE KEY UPDATE shown_at = NOW()`,
		userID, uint32(problemID),
	)
	if err != nil {
		glog.Warningf("%s recordRecentlyShown upsert user=%d problem=%d: %v",
			logPrefix, userID, problemID, err)
	}
}

// loadGamestateAndSettings loads both the gamestate and settings row for a
// user in a single round-trip. Both tables are keyed by user_id and the
// full-path processEvents handler always needs both - combining the fetch
// saves a round-trip per request.
func (a *Api) loadGamestateAndSettings(userID uint32) (*Gamestate, *Settings, error) {
	gs := &Gamestate{}
	s := &Settings{}
	// INNER JOIN: every user that has a gamestate also has a settings row
	// (both are created together in customCreateOrUpdateUser). If either
	// is missing, the row count is 0 and Scan returns sql.ErrNoRows.
	err := a.DB.QueryRow(`
		SELECT
		  g.user_id, g.problem_id, g.video_id, g.solved, g.target,
		  s.problem_type_bitmap, s.target_difficulty, s.target_work_percentage, s.grade_level
		FROM gamestates g
		JOIN settings s ON s.user_id = g.user_id
		WHERE g.user_id = ?`,
		userID,
	).Scan(
		&gs.UserId, &gs.ProblemId, &gs.VideoId, &gs.Solved, &gs.Target,
		&s.ProblemTypeBitmap, &s.TargetDifficulty, &s.TargetWorkPercentage, &s.GradeLevel,
	)
	if err != nil {
		return nil, nil, err
	}
	// Settings.UserId isn't projected (JOIN deduplicates by g.user_id).
	s.UserId = gs.UserId
	return gs, s, nil
}

// createEventsBatch INSERTs N events in a single multi-row INSERT, saving
// N-1 round-trips vs calling eventManager.Create per event. The events
// table has timestamp DEFAULT CURRENT_TIMESTAMP, so we don't need to set
// per-row timestamps. Callers don't use the auto-increment id returned by
// Create, so dropping it is safe.
func (a *Api) createEventsBatch(userID uint32, events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	placeholders := make([]string, len(events))
	args := make([]interface{}, 0, len(events)*3)
	for i, e := range events {
		e.UserId = userID
		placeholders[i] = "(?, ?, ?)"
		args = append(args, e.UserId, e.EventType, e.Value)
	}
	query := "INSERT INTO events (user_id, event_type, value) VALUES " + strings.Join(placeholders, ", ")
	_, err := a.DB.Exec(query, args...)
	return err
}

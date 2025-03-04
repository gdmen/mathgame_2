// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
)

const (
	maxTarget = 20
)

// Do stuff based on the event and write an updated Gamestate / any other side effects
func (a *Api) processEvents(logPrefix string, c *gin.Context, events []*Event, writeCtx bool) error {
	// Get User
	user := GetUserFromContext(c)

	// Get Gamestate
	gamestate, status, msg, err := a.gamestateManager.Get(user.Id)
	if HandleMngrResp(logPrefix, c, status, msg, err, gamestate) != nil {
		return err
	}
	glog.Infof("%s Gamestate: %v", logPrefix, gamestate)

	// Get Settings
	settings, status, msg, err := a.settingsManager.Get(user.Id)
	if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
		return err
	}
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
	changed_problem_settings := false

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
		// TODO: validate
		changed_problem_settings = true
	} else if event.EventType == SET_TARGET_WORK_PERCENTAGE {
		// TODO: validate
	} else if event.EventType == SET_PROBLEM_TYPE_BITMAP {
		// TODO: validate
		changed_problem_settings = true
		a.generateProblemsBackground(logPrefix, c, settings)
	} else if event.EventType == SET_GAMESTATE_TARGET {
		// TODO: validate
	} else if event.EventType == DISPLAYED_PROBLEM {
		// TODO: validate problemID
	} else if event.EventType == WORKING_ON_PROBLEM {
		// TODO: validate duration
	} else if event.EventType == ANSWERED_PROBLEM {
		// Get Problem
		problem, status, msg, err := a.problemManager.Get(gamestate.ProblemId)
		if HandleMngrResp(logPrefix, c, status, msg, err, problem) != nil {
			return err
		}
		if event.Value != problem.Answer {
			msg := fmt.Sprintf("Incorrect answer: {%s}, expected: {%s}", event.Value, problem.Answer)
			glog.Infof("%s %s", logPrefix, msg)
		} else { // Answer was correct
			// Update counts
			gamestate.Solved += 1
			// Select a new problem
			changed_problem_settings = true
		}
	} else if event.EventType == ERROR_PLAYING_VIDEO {
		// Get the current video
		video, status, msg, err := a.videoManager.Get(gamestate.VideoId, user.Id)
		if HandleMngrResp(logPrefix, c, status, msg, err, video) != nil {
			return err
		}
		// Disable the current video
		glog.Infof("%s Disabling video: %v", logPrefix, video)
		video.Disabled = true
		// Save the disabled video
		status, msg, err = a.videoManager.Update(video, user.Id)
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
		// End difficulty adjustment limits

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
			} else {
				changeGamestateTarget(uint32(math.Max(float64(minProbs), math.Ceil(float64(gamestate.Target)/2))))
				changeTargetDifficulty(settings.TargetDifficulty + math.Max(1, diffIncrease*settings.TargetDifficulty))
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
	} else {
		msg := fmt.Sprintf("Invalid EventType: %s", event.EventType)
		glog.Errorf("%s %s", logPrefix, msg)
		c.JSON(http.StatusBadRequest, msg)
		return errors.New(msg)
	}

	// Select a new problem
	if changed_problem_settings {
		// Get the most recent problem ids
		sql := fmt.Sprintf("user_id=%d AND event_type='displayed_problem' AND timestamp >= NOW() - INTERVAL 30 MINUTE;", user.Id)
		glog.Infof("recent problem ids sql: select * from events where %s\n", sql)
		prevProblems, _, msg, err := a.eventManager.CustomList(sql)
		if err != nil {
			glog.Errorf("%s %s", logPrefix, msg)
			c.JSON(http.StatusInternalServerError, msg)
			return err
		}
		problemIds := []uint32{}
		for _, p := range *prevProblems {
			val, err := strconv.ParseUint(p.Value, 10, 32)
			if err != nil {
				continue
			}
			problemIds = append(problemIds, uint32(val))
		}
		problem, err := a.selectProblem(logPrefix, c, settings, &problemIds)
		if err != nil {
			return err
		}
		gamestate.ProblemId = problem.Id
		changed_gamestate = true
	}

	// Write all events to database
	for _, e := range events {
		e.UserId = gamestate.UserId
		e.Timestamp = time.Now()
		status, msg, err := a.eventManager.Create(e)
		if HandleMngrResp(logPrefix, c, status, msg, err, e) != nil {
			return err
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

	// Write the gamestate to the response body
	if writeCtx {
		HandleMngrRespWriteCtx(logPrefix, c, 200, "", nil, gamestate)
	}

	return nil
}

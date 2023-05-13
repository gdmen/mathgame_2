// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/generator"
)

func (a *Api) CustomValueQuery(sql string) (string, int, string, error) {
	var value string
	err := a.DB.QueryRow(sql).Scan(&value)
	if err != nil {
		msg := "Couldn't get value from database"
		return "", http.StatusInternalServerError, msg, err
	}
	return value, http.StatusOK, "", nil
}

func (a *Api) generateProblem(logPrefix string, c *gin.Context, settings *Settings) (*Problem, error) {
	model := &Problem{}
	// TODO: this is temporary logic to make the generator compatible with the new ProblemTypeBitmap
	fractions := false //(FRACTIONS & settings.ProblemTypeBitmap) > 0
	negatives := false //(NEGATIVES & settings.ProblemTypeBitmap) > 0
	operations := []string{}
	if (ADDITION & settings.ProblemTypeBitmap) > 0 {
		operations = append(operations, "+")
	}
	if (SUBTRACTION & settings.ProblemTypeBitmap) > 0 {
		operations = append(operations, "-")
	}
	generator_opts := &generator.Options{
		Operations:       operations,
		Fractions:        fractions,
		Negatives:        negatives,
		TargetDifficulty: settings.TargetDifficulty,
	}

	var err error
	model.Expression, model.Answer, model.Difficulty, err = generator.GenerateProblem(generator_opts)
	if err != nil {
		if err, ok := err.(*generator.OptionsError); ok {
			msg := "Failed options validation"
			glog.Errorf("%s %s: %v", logPrefix, msg, err)
			c.JSON(http.StatusBadRequest, common.GetError(msg))
			return nil, err
		}
		msg := "Couldn't generate problem"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, common.GetError(msg))
		return nil, err
	}
	model.ProblemTypeBitmap = 0
	if strings.Contains(model.Expression, "+") {
		model.ProblemTypeBitmap += ADDITION
	}
	if strings.Contains(model.Expression, "-") {
		model.ProblemTypeBitmap += SUBTRACTION
	}

	// Use expression hash as model.Id
	h := fnv.New32a()
	h.Write([]byte(model.Expression))
	model.Id = h.Sum32()

	// Write to database
	// TODO: collisions here will return the wrong Expre/Ans for the given problem id after returning a 200 for a duplicate?
	status, msg, err := a.problemManager.Create(model)
	if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
		return nil, err
	}

	return model, nil
}

func (a *Api) selectVideo(logPrefix string, c *gin.Context, userId uint32) (uint32, error) {
	if a.isTest {
		return 1, nil
	}

	// Get videos belonging to this user
	videos, status, msg, err := a.videoManager.CustomList(fmt.Sprintf("SELECT * FROM videos WHERE user_id=%d AND disabled=0;", userId))
	if HandleMngrResp(logPrefix, c, status, msg, err, videos) != nil {
		return 0, err
	}

	// If there are no videos for this user, select from all videos
	if len(*videos) < 1 {
		videos, status, msg, err = a.videoManager.CustomList("SELECT * FROM videos WHERE disabled=0;")
		if HandleMngrResp(logPrefix, c, status, msg, err, videos) != nil {
			return 0, err
		}
	}

	var videoIds []uint32
	for _, v := range *videos {
		videoIds = append(videoIds, v.Id)
	}

	// If there are no videos at all in the database, add a default and use that
	if len(videoIds) < 1 {
		msg := "Couldn't find any videos in the database, adding a default."
		glog.Errorf("%s %s", logPrefix, msg)
		video := &Video{
			Title:    "You've Got a Friend in Me",
			URL:      "https://www.youtube.com/watch?v=nMN4JZ8crVY",
			Disabled: false,
		}
		status, msg, err := a.videoManager.Create(video)
		if HandleMngrResp(logPrefix, c, status, msg, err, video) != nil {
			return 0, err
		}
		videoIds = append(videoIds, video.Id)
	}

	// Select video
	ind := rand.Intn(len(videoIds))
	videoId := videoIds[ind]

	return videoId, nil
}

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
	} else if event.EventType == SET_TARGET_WORK_PERCENTAGE {
		// TODO: validate
	} else if event.EventType == SET_PROBLEM_TYPE_BITMAP {
		// TODO: validate
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
			// Generate a new problem
			problem, err := a.generateProblem(logPrefix, c, settings)
			if err != nil {
				return err
			}
			gamestate.ProblemId = problem.Id
			changed_gamestate = true
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
		videoId, err := a.selectVideo(logPrefix, c, user.Id)
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
		var maxProbs uint32 = 30
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
			if gamestate.Target < maxProbs {
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
		videoId, err := a.selectVideo(logPrefix, c, user.Id)
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

// also generate settings change events
func (a *Api) customUpdateSettings(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Settings{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Get User
	user := GetUserFromContext(c)

	// Get Settings
	settings, status, msg, err := a.settingsManager.Get(user.Id)
	if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
		return
	}

	// Write to database
	status, msg, err = a.settingsManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}

	// Trigger events for all the changed settings
	var events []*Event
	if model.ProblemTypeBitmap != settings.ProblemTypeBitmap {
		events = append(events, &Event{
			EventType: SET_PROBLEM_TYPE_BITMAP,
			Value:     strconv.FormatUint(model.ProblemTypeBitmap, 10),
		})
	}
	if model.TargetDifficulty != settings.TargetDifficulty {
		events = append(events, &Event{
			EventType: SET_TARGET_DIFFICULTY,
			Value:     strconv.FormatFloat(model.TargetDifficulty, 'E', -1, 64),
		})
	}
	if model.TargetWorkPercentage != settings.TargetWorkPercentage {
		events = append(events, &Event{
			EventType: SET_TARGET_WORK_PERCENTAGE,
			Value:     strconv.FormatUint(uint64(model.TargetWorkPercentage), 10),
		})
	}
	if a.processEvents(logPrefix, c, events, false) != nil {
		return
	}
}

func (a *Api) customGetNumEnabledVideos(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Get User
	user := GetUserFromContext(c)

	// Get a count of enabled videos for this user
	sql := fmt.Sprintf("SELECT count(*) FROM videos WHERE user_id=%d AND disabled=0;", user.Id)
	value, status, msg, err := a.CustomValueQuery(sql)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, value) != nil {
		return
	}
}

func (a *Api) customCreateEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	event := &Event{}
	if BindModelFromForm(logPrefix, c, event) != nil {
		return
	}
	glog.Infof("%s bound model: %v", logPrefix, event)

	if a.processEvents(logPrefix, c, []*Event{event}, true) != nil {
		return
	}
}

func (a *Api) customListEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	type Params struct {
		UserId  uint32 `json:"user_id" uri:"user_id" form:"user_id"`
		Seconds uint32 `json:"seconds" uri:"seconds" form:"seconds"`
	}
	params := &Params{}
	if BindModelFromURI(logPrefix, c, params) != nil {
		return
	}

	// Get recent events belonging to the specified user
	sql := fmt.Sprintf("SELECT * FROM events WHERE user_id=%d AND timestamp > now() - interval %d second AND event_type IN (\"%s\");", params.UserId, params.Seconds, strings.Join([]string{LOGGED_IN, DISPLAYED_PROBLEM, ANSWERED_PROBLEM, DONE_WATCHING_VIDEO}, "\",\""))
	events, status, msg, err := a.eventManager.CustomList(sql)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, events) != nil {
		return
	}
}

// If the current reward video is deleted, we need to replace it
func (a *Api) customDeleteVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Get User
	user := GetUserFromContext(c)

	// Get Gamestate
	gamestate, status, msg, err := a.gamestateManager.Get(user.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, gamestate) != nil {
		return
	}
	glog.Infof("%s Gamestate: %v", logPrefix, gamestate)

	if gamestate.VideoId == model.Id {
		// Set a new reward video
		videoId, err := a.selectVideo(logPrefix, c, user.Id)
		if err != nil {
			return
		}
		gamestate.VideoId = videoId
		// Write the updated gamestate
		glog.Infof("%s Gamestate: %v", logPrefix, gamestate)
		status, msg, err = a.gamestateManager.Update(gamestate)
		if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, gamestate) != nil {
			return
		}
	}

	// Write video to database
	status, msg, err = a.videoManager.Delete(model.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) customCreateOrUpdateUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	user := &User{}
	if BindModelFromForm(logPrefix, c, user) != nil {
		return
	}

	// Write user to database
	user.Auth0Id = GetAuth0IdFromContext(c)
	status, msg, err := a.userManager.Create(user)
	if status != http.StatusCreated {
		if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, user) != nil {
			return
		}
	} else { // user was newly created
		user, status, msg, err = a.userManager.Get(user.Auth0Id)
		if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, user) != nil {
			return
		}
		// Write default new settings to database
		const default_problem_type_bitmap uint64 = 0
		const default_target_difficulty float64 = 3
		const default_target_work_percentage uint8 = 70
		const default_gamestate_target uint32 = 10
		default_settings := &Settings{
			UserId:               user.Id,
			ProblemTypeBitmap:    default_problem_type_bitmap,
			TargetDifficulty:     default_target_difficulty,
			TargetWorkPercentage: default_target_work_percentage,
		}
		status, msg, err := a.settingsManager.Create(default_settings)
		if HandleMngrResp(logPrefix, c, status, msg, err, default_settings) != nil {
			return
		}
		// Get settings
		settings, status, msg, err := a.settingsManager.Get(user.Id)
		if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
			return
		}
		glog.Infof("%s Settings: %v", logPrefix, settings)
		// Generate a new problem
		problem, err := a.generateProblem(logPrefix, c, settings)
		if err != nil {
			return
		}
		// Set a new reward video
		videoId, err := a.selectVideo(logPrefix, c, user.Id)
		if err != nil {
			return
		}

		// Write default new gamestate to database
		default_gamestate := &Gamestate{
			UserId:    user.Id,
			ProblemId: problem.Id,
			VideoId:   videoId,
			Solved:    0,
			Target:    default_gamestate_target,
		}
		status, msg, err = a.gamestateManager.Create(default_gamestate)
		if HandleMngrResp(logPrefix, c, status, msg, err, default_gamestate) != nil {
			return
		}

		// Trigger events for all the new settings
		var events []*Event
		events = append(events, &Event{
			UserId:    user.Id,
			EventType: SET_PROBLEM_TYPE_BITMAP,
			Value:     strconv.FormatUint(default_problem_type_bitmap, 10),
		})
		events = append(events, &Event{
			UserId:    user.Id,
			EventType: SET_TARGET_DIFFICULTY,
			Value:     strconv.FormatFloat(default_target_difficulty, 'E', -1, 64),
		})
		events = append(events, &Event{
			UserId:    user.Id,
			EventType: SET_TARGET_WORK_PERCENTAGE,
			Value:     strconv.FormatUint(uint64(default_target_work_percentage), 10),
		})
		events = append(events, &Event{
			UserId:    user.Id,
			EventType: SET_GAMESTATE_TARGET,
			Value:     strconv.FormatUint(uint64(default_gamestate_target), 10),
		})
		if a.processEvents(logPrefix, c, events, false) != nil {
			return
		}

	}

	event := &Event{
		UserId:    user.Id,
		EventType: LOGGED_IN,
	}

	if a.processEvents(logPrefix, c, []*Event{event}, false) != nil {
		return
	}
}

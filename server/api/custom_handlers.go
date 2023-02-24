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

const (
	// EventTypes
	LOGGED_IN           = "logged_in"           // no value
	DISPLAYED_PROBLEM   = "displayed_problem"   // int ProblemID
	WORKING_ON_PROBLEM  = "working_on_problem"  // int Duration in seconds
	ANSWERED_PROBLEM    = "answered_problem"    // string Answer
	WATCHING_VIDEO      = "watching_video"      // int Duration in seconds
	DONE_WATCHING_VIDEO = "done_watching_video" // int VideoID
	// -end- EventTypes
)

var EventTypes = [...]string{
	LOGGED_IN,
	DISPLAYED_PROBLEM,
	WORKING_ON_PROBLEM,
	ANSWERED_PROBLEM,
	WATCHING_VIDEO,
	DONE_WATCHING_VIDEO,
}

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
	generator_opts := &generator.Options{
		Operations:       strings.Split(settings.Operations, ","),
		Fractions:        settings.Fractions,
		Negatives:        settings.Negatives,
		TargetDifficulty: settings.TargetDifficulty,
	}

	var err error
	model.Expression, model.Answer, model.Difficulty, err = generator.GenerateProblem(generator_opts)
	if err != nil {
		if err, ok := err.(*generator.OptionsError); ok {
			msg := "Failed options validation"
			glog.Errorf("%s %s: %v", logPrefix, msg, err)
			c.JSON(http.StatusBadRequest, GetError(msg))
			return nil, err
		}
		msg := "Couldn't generate problem"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
		return nil, err
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
	videos, status, msg, err := a.videoManager.CustomList(fmt.Sprintf("SELECT * FROM videos INNER JOIN userHasVideos ON videos.id = userHasVideos.video_id WHERE userHasVideos.user_id=%d AND videos.enabled=1;", userId))
	if HandleMngrResp(logPrefix, c, status, msg, err, videos) != nil {
		return 0, err
	}

	// If there are no videos for this user, select from all videos
	if len(*videos) < 1 {
		videos, status, msg, err = a.videoManager.CustomList("SELECT * FROM videos WHERE enabled=1;")
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
			Title:   "You've Got a Friend in Me",
			URL:     "https://www.youtube.com/watch?v=rUWxSEwctFU", //"https://www.youtube.com/watch?v=nMN4JZ8crVY",
			Start:   0,
			End:     9999,
			Enabled: true,
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

// Do stuff based on the event and return an updated Gamestate{}
func (a *Api) processEvent(logPrefix string, c *gin.Context, event *Event, writeCtx bool) error {
	auth0Id := GetAuth0IdFromContext(logPrefix, c, a.isTest)

	// Get User
	user, status, msg, err := a.userManager.Get(auth0Id)
	if HandleMngrResp(logPrefix, c, status, msg, err, user) != nil {
		return err
	}

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

	if event.EventType == LOGGED_IN {
		// no-op
	} else if event.EventType == DISPLAYED_PROBLEM {
		// TODO: validate problemID
	} else if event.EventType == WORKING_ON_PROBLEM {
		// TODO: valudate duration
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
		}
	} else if event.EventType == WATCHING_VIDEO {
		// TODO: validate duration
	} else if event.EventType == DONE_WATCHING_VIDEO {
		// TODO: validate videoID

		// Difficulty adjustment limits
		workTarget := 0.70
		epsilon := 0.05
		var recentPast int = 900 // seconds aka 15 minutes. This assumes a 1 second event reporting interval.
		var minDiff float64 = 3
		var maxProbs uint32 = 20
		var minProbs uint32 = 5
		// End difficulty adjustment limits

		if gamestate.Solved >= gamestate.Target {
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
			workPercentage, err := strconv.ParseFloat(value, 32)
			glog.Infof("%s workPercentage: %v", logPrefix, workPercentage)
			if err != nil {
				return err
			}
			// Adjust work load. Levers are difficulty and target number of problems.
			glog.Infof("%s workTarget: %v", logPrefix, workTarget)

			glog.Infof("%s starting difficulty & num problems: %v, %v", logPrefix, settings.TargetDifficulty, gamestate.Target)
			// Only do something if we are not already on target
			if math.Abs(workTarget-workPercentage) < epsilon {
				glog.Infof("%s difficulty is on target", logPrefix)
			} else if workTarget > workPercentage {
				// Make it more difficult
				if gamestate.Target < maxProbs {
					gamestate.Target += 1
				} else {
					gamestate.Target -= 1
					settings.TargetDifficulty += math.Max(0.1*settings.TargetDifficulty, 1)
				}
			} else if workTarget < workPercentage {
				// Make it easier
				if gamestate.Target > minProbs {
					gamestate.Target -= 1
				} else {
					gamestate.Target += 1
					if settings.TargetDifficulty <= minDiff {
						glog.Infof("%s difficulty of %v should not be below %v and we're already at minProbs. Will not make this easier.", logPrefix, settings.TargetDifficulty, minDiff)
						settings.TargetDifficulty = minDiff
					} else {
						settings.TargetDifficulty = math.Max(minDiff, settings.TargetDifficulty-math.Max(0.1*settings.TargetDifficulty, 1))
					}
				}
			}
			glog.Infof("%s modified difficulty & num problems: %v, %v", logPrefix, settings.TargetDifficulty, gamestate.Target)

			// Reset solved progress
			gamestate.Solved = 0

			// Set a new reward video
			videoId, err := a.selectVideo(logPrefix, c, user.Id)
			if err != nil {
				return err
			}
			gamestate.VideoId = videoId
		}
	} else {
		msg := fmt.Sprintf("Invalid EventType: %s", event.EventType)
		glog.Errorf("%s %s", logPrefix, msg)
		c.JSON(http.StatusBadRequest, msg)
		return errors.New(msg)
	}

	// Write event to database
	event.UserId = gamestate.UserId
	event.Timestamp = time.Now()
	status, msg, err = a.eventManager.Create(event)
	if HandleMngrResp(logPrefix, c, status, msg, err, event) != nil {
		return err
	}

	// Write the updated settings
	glog.Infof("%s Option: %v", logPrefix, settings)
	status, msg, err = a.settingsManager.Update(settings)
	if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
		return err
	}

	// Write the updated gamestate
	glog.Infof("%s Gamestate: %v", logPrefix, gamestate)
	status, msg, err = a.gamestateManager.Update(gamestate)
	if (writeCtx && HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, gamestate) != nil) || HandleMngrResp(logPrefix, c, status, msg, err, gamestate) != nil {
		return err
	}

	return nil
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

	if a.processEvent(logPrefix, c, event, true) != nil {
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
	glog.Infof("%s HERE!", logPrefix)
	glog.Infof(fmt.Sprintf("SELECT * FROM events WHERE user_id=%d AND timestamp > now() - interval %d second AND event_type IN (%s);", params.UserId, params.Seconds, strings.Join([]string{LOGGED_IN, DISPLAYED_PROBLEM, ANSWERED_PROBLEM, DONE_WATCHING_VIDEO}, ",")))
	events, status, msg, err := a.eventManager.CustomList(fmt.Sprintf("SELECT * FROM events WHERE user_id=%d AND timestamp > now() - interval %d second AND event_type IN (\"%s\");", params.UserId, params.Seconds, strings.Join([]string{LOGGED_IN, DISPLAYED_PROBLEM, ANSWERED_PROBLEM, DONE_WATCHING_VIDEO}, "\",\"")))
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, events) != nil {
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
	user.Auth0Id = GetAuth0IdFromContext(logPrefix, c, a.isTest)
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
		default_settings := &Settings{UserId: user.Id}
		status, msg, err := a.settingsManager.Create(default_settings)
		if HandleMngrResp(logPrefix, c, status, msg, err, default_settings) != nil {
			return
		}
		// Get settings
		settings, status, msg, err := a.settingsManager.Get(user.Id)
		if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
			return
		}
		glog.Infof("%s Option: %v", logPrefix, settings)
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
			Target:    20,
		}
		status, msg, err = a.gamestateManager.Create(default_gamestate)
		if HandleMngrResp(logPrefix, c, status, msg, err, default_gamestate) != nil {
			return
		}
	}

	event := &Event{
		UserId:    user.Id,
		EventType: LOGGED_IN,
	}

	if a.processEvent(logPrefix, c, event, false) != nil {
		return
	}
}

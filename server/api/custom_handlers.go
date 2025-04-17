// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

const (
	nullVideoId = math.MaxUint32
)

func (a *Api) selectVideo(logPrefix string, c *gin.Context, userId uint32, exclusions map[uint32]bool) (uint32, error) {
	// Get videos belonging to this user
	videos, status, msg, err := a.videoManager.CustomList(fmt.Sprintf("user_id=%d AND disabled=0 AND deleted=0;", userId))
	if HandleMngrResp(logPrefix, c, status, msg, err, videos) != nil {
		return 0, err
	}

	var videoIds []uint32
	for _, v := range *videos {
		if _, ok := exclusions[v.Id]; ok {
			continue
		}
		videoIds = append(videoIds, v.Id)
	}

	// If there are no videos at all in the database, do nothing
	if len(videoIds) < 1 {
		msg := fmt.Sprintf("Couldn't find any videos for this user (%d): silently do nothing.", userId)
		glog.Errorf("%s %s", logPrefix, msg)
		return nullVideoId, nil
	}

	// Select video
	ind := rand.Intn(len(videoIds))
	videoId := videoIds[ind]

	return videoId, nil
}

func (a *Api) selectVideoIfNull(logPrefix string, c *gin.Context, gamestate *Gamestate, writeCtx bool) error {
	var status int
	var msg string
	var err error
	// Select a video if setup is done and no video is already selected
	if gamestate.VideoId == nullVideoId {
		videoId, err := a.selectVideo(logPrefix, c, gamestate.UserId, map[uint32]bool{})
		if err != nil {
			return err
		}
		gamestate.VideoId = videoId
		status, msg, err = a.gamestateManager.Update(gamestate)
	}
	handleFcn := HandleMngrResp
	if writeCtx {
		handleFcn = HandleMngrRespWriteCtx
	}
	if handleFcn(logPrefix, c, status, msg, err, gamestate) != nil {
		return err
	}
	return nil
}

func (a *Api) customGetGamestate(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Gamestate{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	model, status, msg, err := a.gamestateManager.Get(model.UserId)
	if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}

	a.selectVideoIfNull(logPrefix, c, model, true)
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

// Get all user info that the client uses on page load
func (a *Api) customGetPageLoadData(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Get User
	user := &User{}
	if BindModelFromURI(logPrefix, c, user) != nil {
		return
	}
	// Read from database
	user, status, msg, err := a.userManager.Get(user.Auth0Id)
	if HandleMngrResp(logPrefix, c, status, msg, err, user) != nil {
		return
	}

	// Get Settings
	settings, status, msg, err := a.settingsManager.Get(user.Id)
	if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
		return
	}

	// Get a count of enabled videos for this user
	sql := fmt.Sprintf("SELECT count(*) FROM videos WHERE user_id=%d AND disabled=0 AND deleted=0;", user.Id)
	value, status, msg, err := a.CustomValueQuery(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, value) != nil {
		return
	}

	// Write out the data
	data := PageLoadData{
		User:             user,
		Settings:         settings,
		NumVideosEnabled: value,
	}
	HandleMngrRespWriteCtx(logPrefix, c, http.StatusOK, "", nil, data)
}

// Get all user info that the client uses on the play page
func (a *Api) customGetPlayData(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	gamestate := &Gamestate{}
	if BindModelFromURI(logPrefix, c, gamestate) != nil {
		return
	}

	// Read from database
	gamestate, status, msg, err := a.gamestateManager.Get(gamestate.UserId)
	if HandleMngrResp(logPrefix, c, status, msg, err, gamestate) != nil {
		return
	}

	a.helpGetPlayData(logPrefix, c, gamestate)
}

func (a *Api) helpGetPlayData(logPrefix string, c *gin.Context, gamestate *Gamestate) {
	// Get Problem
	problem, status, msg, err := a.problemManager.Get(gamestate.ProblemId)
	if HandleMngrResp(logPrefix, c, status, msg, err, problem) != nil {
		return
	}

	err = a.selectVideoIfNull(logPrefix, c, gamestate, false)
	if err != nil {
		return
	}

	// Get Video
	video, status, msg, err := a.videoManager.Get(gamestate.VideoId, gamestate.UserId)
	if HandleMngrResp(logPrefix, c, status, msg, err, video) != nil {
		return
	}

	// Write out the data
	data := PlayData{
		Gamestate: gamestate,
		Problem:   problem,
		Video:     video,
	}
	HandleMngrRespWriteCtx(logPrefix, c, http.StatusOK, "", nil, data)
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
	sql := fmt.Sprintf("user_id=%d AND timestamp > now() - interval %d second AND event_type IN (\"%s\");", params.UserId, params.Seconds, strings.Join([]string{LOGGED_IN, SELECTED_PROBLEM, ANSWERED_PROBLEM, SOLVED_PROBLEM, DONE_WATCHING_VIDEO}, "\",\""))
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
		videoId, err := a.selectVideo(logPrefix, c, user.Id, map[uint32]bool{gamestate.VideoId: true})
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
	sql := fmt.Sprintf("UPDATE videos SET deleted=1 WHERE id=%d AND user_id=%d;", model.Id, user.Id)
	status, msg, err = a.videoManager.CustomSql(sql)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) customListVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	user := GetUserFromContext(c)

	// Read from database
	sql := fmt.Sprintf("user_id=%d AND deleted=0;", user.Id)
	models, status, msg, err := a.videoManager.CustomList(sql)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
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

	// If ctx_user is not nil, the user already exists in our database
	ctx_user := GetUserFromContextLenient(c)
	if ctx_user != nil {
		user.Auth0Id = ctx_user.Auth0Id
	} else {
		user.Auth0Id = GetAuth0IdFromContext(c)
	}
	// Write user to database
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
		SetUserInContext(c, user)
		// Write default new settings to database
		const default_problem_type_bitmap uint64 = 1
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
		// Select a new problem
		problem, err := a.selectProblem(logPrefix, c, settings, &([]uint32{0}))
		if err != nil {
			return
		}

		// Write default new gamestate to database
		default_gamestate := &Gamestate{
			UserId:    user.Id,
			ProblemId: problem.Id,
			VideoId:   nullVideoId, // the user hasn't added videos yet
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
		events = append(events, &Event{
			UserId:    user.Id,
			EventType: SELECTED_PROBLEM,
			Value:     strconv.FormatUint(uint64(problem.Id), 10),
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

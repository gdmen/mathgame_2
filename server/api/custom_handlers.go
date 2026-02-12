// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
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
	rows, err := a.DB.Query(`
		SELECT uhv.video_id FROM user_has_video uhv
		INNER JOIN videos v ON v.id = uhv.video_id AND v.disabled = 0
		WHERE uhv.user_id = ?`, userId)
	if err != nil {
		glog.Errorf("%s selectVideo user_has_video: %v", logPrefix, err)
		return 0, err
	}
	defer rows.Close()
	var videoIds []uint32
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			glog.Errorf("%s selectVideo scan: %v", logPrefix, err)
			return 0, err
		}
		if _, ok := exclusions[id]; ok {
			continue
		}
		videoIds = append(videoIds, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
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

	// Get a count of enabled videos for this user (from user_has_video / playlists)
	sql := fmt.Sprintf("SELECT COUNT(*) FROM user_has_video uhv INNER JOIN videos v ON v.id = uhv.video_id AND v.disabled = 0 WHERE uhv.user_id = %d;", user.Id)
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

func (a *Api) countEnabledVideosForUser(userId uint32) (int, error) {
	var n int
	err := a.DB.QueryRow(`
		SELECT COUNT(*) FROM user_has_video uhv
		INNER JOIN videos v ON v.id = uhv.video_id AND v.disabled = 0
		WHERE uhv.user_id = ?`, userId).Scan(&n)
	return n, err
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

	// Require at least 3 videos to play
	count, err := a.countEnabledVideosForUser(gamestate.UserId)
	if err != nil {
		glog.Errorf("%s countEnabledVideosForUser: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not check video count"))
		return
	}
	if count < 3 {
		c.JSON(http.StatusForbidden, common.GetError("Add at least 3 videos via playlists in Settings to play."))
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
	if err != nil || status == http.StatusNotFound || gamestate.ProblemId == 0 {
		// Problem missing or invalid (e.g. id 0); select a new problem and persist it
		glog.Infof("%s problem not found or invalid (id=%d), selecting new problem", logPrefix, gamestate.ProblemId)
		settings, status, msg, err := a.settingsManager.Get(gamestate.UserId)
		if HandleMngrResp(logPrefix, c, status, msg, err, settings) != nil {
			return
		}
		problem, err = a.selectProblem(logPrefix, c, settings, &[]uint32{})
		if err != nil {
			glog.Errorf("%s selectProblem: %v", logPrefix, err)
			c.JSON(http.StatusInternalServerError, common.GetError("Could not select problem"))
			return
		}
		gamestate.ProblemId = problem.Id
		status, msg, err = a.gamestateManager.Update(gamestate)
		if HandleMngrResp(logPrefix, c, status, msg, err, gamestate) != nil {
			return
		}
	} else if HandleMngrResp(logPrefix, c, status, msg, err, problem) != nil {
		return
	}

	err = a.selectVideoIfNull(logPrefix, c, gamestate, false)
	if err != nil {
		return
	}

	// Get Video (by id only; videos table no longer has user_id)
	video := &Video{}
	err = a.DB.QueryRow("SELECT id, title, url, thumbnailurl, you_tube_id, disabled FROM videos WHERE id=?", gamestate.VideoId).
		Scan(&video.Id, &video.Title, &video.URL, &video.ThumbnailURL, &video.YouTubeId, &video.Disabled)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, common.GetError("Video not found"))
			return
		}
		glog.Errorf("%s get video: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not get video"))
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

// Remove a video from the current user's allowed list only. We never delete or
// soft-delete video rows, so event and gamestate references remain valid.
func (a *Api) customDeleteVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	model := &Video{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	user := GetUserFromContext(c)

	result, err := a.DB.Exec("DELETE FROM user_has_video WHERE user_id=? AND video_id=?", user.Id, model.Id)
	if err != nil {
		glog.Errorf("%s remove from user_has_video: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not remove video"))
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, common.GetError("Video not in your list"))
		return
	}

	gamestate, status, msg, err := a.gamestateManager.Get(user.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, gamestate) != nil {
		return
	}
	if gamestate.VideoId == model.Id {
		videoId, err := a.selectVideo(logPrefix, c, user.Id, map[uint32]bool{gamestate.VideoId: true})
		if err != nil {
			return
		}
		gamestate.VideoId = videoId
		status, msg, err = a.gamestateManager.Update(gamestate)
		if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, gamestate) != nil {
			return
		}
	}

	c.Status(http.StatusNoContent)
}

func (a *Api) customCreateVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, common.GetError("unauthorized"))
		return
	}
	model := &Video{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	status, msg, err := a.videoManager.Create(model)
	if err != nil {
		if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
			return
		}
		return
	}
	_, err = a.DB.Exec("INSERT IGNORE INTO user_has_video (user_id, video_id) VALUES (?, ?)", user.Id, model.Id)
	if err != nil {
		glog.Errorf("%s insert user_has_video: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not add video to your list"))
		return
	}
	HandleMngrRespWriteCtx(logPrefix, c, status, "", nil, model)
}

func (a *Api) customListVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	user := GetUserFromContext(c)

	rows, err := a.DB.Query(`
		SELECT v.id, v.title, v.url, v.thumbnailurl, v.you_tube_id, v.disabled
		FROM videos v
		INNER JOIN user_has_video uhv ON uhv.video_id = v.id
		WHERE uhv.user_id = ?`, user.Id)
	if err != nil {
		glog.Errorf("%s list videos: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not list videos"))
		return
	}
	defer rows.Close()
	var models []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.Id, &v.Title, &v.URL, &v.ThumbnailURL, &v.YouTubeId, &v.Disabled); err != nil {
			glog.Errorf("%s scan video: %v", logPrefix, err)
			c.JSON(http.StatusInternalServerError, common.GetError("Could not list videos"))
			return
		}
		models = append(models, v)
	}
	if models == nil {
		models = []Video{}
	}
	c.JSON(http.StatusOK, models)
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
			glog.Errorf("%s Error: %v", logPrefix, err)
			return
		}
		glog.Infof("%s Problem: %v", logPrefix, problem)

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
			glog.Errorf("%s Error: %v", logPrefix, err)
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
			glog.Errorf("%s Error: %v", logPrefix, err)
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

func (a *Api) refreshUserHasVideo(userId uint32) error {
	_, err := a.DB.Exec("DELETE FROM user_has_video WHERE user_id=?", userId)
	if err != nil {
		return err
	}
	_, err = a.DB.Exec(`
		INSERT INTO user_has_video (user_id, video_id)
		SELECT DISTINCT up.user_id, pv.video_id
		FROM user_playlist up
		INNER JOIN playlist_video pv ON up.playlist_id = pv.playlist_id
		WHERE up.user_id = ?`,
		userId)
	return err
}

func (a *Api) customListPlaylists(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, common.GetError("unauthorized"))
		return
	}
	rows, err := a.DB.Query(`
		SELECT p.id, p.you_tube_id, p.title, p.thumbnailurl, p.etag
		FROM playlists p
		INNER JOIN user_playlist up ON p.id = up.playlist_id
		WHERE up.user_id = ?`,
		user.Id)
	if err != nil {
		glog.Errorf("%s list my playlists: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not list playlists"))
		return
	}
	defer rows.Close()
	var list []Playlist
	for rows.Next() {
		var p Playlist
		err := rows.Scan(&p.Id, &p.YouTubeId, &p.Title, &p.ThumbnailURL, &p.Etag)
		if err != nil {
			glog.Errorf("%s scan playlist: %v", logPrefix, err)
			c.JSON(http.StatusInternalServerError, common.GetError("Could not list playlists"))
			return
		}
		list = append(list, p)
	}
	if err = rows.Err(); err != nil {
		glog.Errorf("%s rows.Err: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not list playlists"))
		return
	}
	if list == nil {
		list = []Playlist{}
	}
	c.JSON(http.StatusOK, list)
}

func extractPlaylistIDFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return u.Query().Get("list")
	}
	return raw
}

func (a *Api) customAddPlaylist(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, common.GetError("unauthorized"))
		return
	}
	var body struct {
		PlaylistID        *uint32 `json:"playlist_id"`
		YouTubePlaylistID *string `json:"youtube_playlist_id"`
		PlaylistURL       *string `json:"playlist_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, common.GetError("Invalid request body"))
		return
	}
	var playlistID uint32
	if body.PlaylistID != nil {
		_, status, msg, err := a.playlistManager.Get(*body.PlaylistID)
		if err != nil {
			glog.Errorf("%s get playlist: %v", logPrefix, err)
			c.JSON(status, common.GetError(msg))
			return
		}
		playlistID = *body.PlaylistID
	} else {
		var ytID string
		if body.YouTubePlaylistID != nil && *body.YouTubePlaylistID != "" {
			ytID = strings.TrimSpace(*body.YouTubePlaylistID)
		} else if body.PlaylistURL != nil {
			ytID = extractPlaylistIDFromURL(*body.PlaylistURL)
		}
		if ytID == "" {
			c.JSON(http.StatusBadRequest, common.GetError("Provide playlist_id, youtube_playlist_id, or playlist_url"))
			return
		}
		syncedID, err := a.syncPlaylistFromYouTube(ytID)
		if err != nil {
			glog.Errorf("%s syncPlaylistFromYouTube %s: %v", logPrefix, ytID, err)
			c.JSON(http.StatusBadRequest, common.GetError("Could not fetch playlist from YouTube: "+err.Error()))
			return
		}
		playlistID = syncedID
	}
	_, err := a.DB.Exec("INSERT IGNORE INTO user_playlist (user_id, playlist_id) VALUES (?, ?)", user.Id, playlistID)
	if err != nil {
		glog.Errorf("%s add user_playlist: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not add playlist"))
		return
	}
	if err := a.refreshUserHasVideo(user.Id); err != nil {
		glog.Errorf("%s refreshUserHasVideo: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not update video list"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": playlistID})
}

func (a *Api) customRemovePlaylist(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, common.GetError("unauthorized"))
		return
	}
	var uri struct {
		PlaylistID uint32 `uri:"playlist_id" binding:"required"`
	}
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, common.GetError("Invalid playlist_id"))
		return
	}
	result, err := a.DB.Exec("DELETE FROM user_playlist WHERE user_id=? AND playlist_id=?", user.Id, uri.PlaylistID)
	if err != nil {
		glog.Errorf("%s remove user_playlist: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not remove playlist"))
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, common.GetError("Playlist not in your list"))
		return
	}
	if err := a.refreshUserHasVideo(user.Id); err != nil {
		glog.Errorf("%s refreshUserHasVideo: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not update video list"))
		return
	}
	c.Status(http.StatusNoContent)
}

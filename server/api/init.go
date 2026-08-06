// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	gin_adapter "github.com/gwatts/gin-adapter"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/auth0"
	"garydmenezes.com/mathgame/server/mathcore"
)

const (
	CreatePlaylistVideoTableSQL = `
CREATE TABLE playlist_video (
    playlist_id BIGINT UNSIGNED NOT NULL,
    video_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (playlist_id, video_id),
    FOREIGN KEY (playlist_id) REFERENCES playlists(id),
    FOREIGN KEY (video_id) REFERENCES videos(id)
) DEFAULT CHARSET=utf8mb4 ;`
	CreateUserPlaylistTableSQL = `
CREATE TABLE user_playlist (
    user_id BIGINT UNSIGNED NOT NULL,
    playlist_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (user_id, playlist_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (playlist_id) REFERENCES playlists(id)
) DEFAULT CHARSET=utf8mb4 ;`
	CreateUserHasVideoTableSQL = `
CREATE TABLE user_has_video (
    user_id BIGINT UNSIGNED NOT NULL,
    video_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (user_id, video_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (video_id) REFERENCES videos(id)
) DEFAULT CHARSET=utf8mb4 ;`
)

var CREATE_TABLES_SQL = []string{
	CreateUserTableSQL,
	CreateVideoTableSQL,
	CreateProblemTableSQL,
	CreateSettingsTableSQL,
	CreateGamestateTableSQL,
	CreateEventTableSQL,
	CreatePlaylistTableSQL,
	CreatePlaylistVideoTableSQL,
	CreateUserPlaylistTableSQL,
	CreateUserHasVideoTableSQL,
}

type Api struct {
	DB                          *sql.DB
	YouTubeAPIKey               string
	auth0Domain                 string
	auth0ManagementClientId     string
	auth0ManagementClientSecret string
	auth0ManagementDomain       string
	isTest                      bool
	userManager                 *UserManager
	videoManager                *VideoManager
	problemManager              *ProblemManager
	settingsManager             *SettingsManager
	gamestateManager            *GamestateManager
	eventManager                *EventManager
	playlistManager             *PlaylistManager

	// bitmapMatrixBitmaps overrides the bitmap universe the coverage-matrix
	// recompute walks. nil = the full mathcore.EnumerateValidBitmaps() space;
	// tests inject a tiny slice so the recompute stays fast.
	bitmapMatrixBitmaps []mathcore.ProblemType
}

// createTables asserts the base tables at their current generated shape,
// treating "already exists" as success.
func createTables(db *sql.DB) error {
	for _, sql := range CREATE_TABLES_SQL {
		_, err := db.Exec(sql)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				msg := "Not creating table"
				glog.Infof("%s: %v", msg, err)
				continue
			}
			return err
		}
	}
	return nil
}

func NewApi(db *sql.DB, cfg *common.Config) (*Api, error) {
	if err := createTables(db); err != nil {
		return nil, err
	}
	a := &Api{DB: db}
	if cfg != nil {
		a.YouTubeAPIKey = cfg.YouTubeAPIKey
		a.auth0Domain = cfg.Auth0Domain
		a.auth0ManagementClientId = cfg.Auth0ManagementClientId
		a.auth0ManagementClientSecret = cfg.Auth0ManagementClientSecret
		a.auth0ManagementDomain = cfg.Auth0ManagementDomain
	}
	a.userManager = &UserManager{DB: db}
	a.videoManager = &VideoManager{DB: db}
	a.problemManager = &ProblemManager{DB: db}
	a.settingsManager = &SettingsManager{DB: db}
	a.gamestateManager = &GamestateManager{DB: db}
	a.eventManager = &EventManager{DB: db}
	a.playlistManager = &PlaylistManager{DB: db}
	return a, nil
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

func (a *Api) genGetUserFn() common.GetUserFn {
	return func(logPrefix string, c *gin.Context) (interface{}, error) {
		user, status, msg, err := a.userManager.Get(c.MustGet(common.Auth0IdKey).(string))
		// We don't use the standard handler helpers here because we don't want to write error statuses if we don't find the user.
		if err != nil {
			glog.Infof("%s %d: %s", logPrefix, status, msg)
		}
		return user, err
	}
}

func (a *Api) GetRouter() *gin.Engine {
	router := gin.Default()
	// - No origin allowed by default
	// - GET,POST, PUT, HEAD methods
	// - Credentials share disabled
	// - Preflight requests cached for 12 hours
	config := cors.DefaultConfig()
	// TODO: limit allowed origins
	config.AllowAllOrigins = true
	config.AllowHeaders = append(config.AllowHeaders, "Authorization")
	// The bitmap-matrix GET carries its report metadata in response headers (the
	// body is the raw gzipped report); expose them so cross-origin JS can read
	// the computing/progress state.
	config.ExposeHeaders = []string{"X-Has-Report", "X-Computing", "X-Compute-Done", "X-Compute-Total", "X-Computed-At"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	router.Use(cors.New(config))

	// Use our request id middleware
	router.Use(common.RequestIdMiddleware())

	// Use our auth0 jwt middleware
	if !a.isTest {
		router.Use(gin_adapter.Wrap(auth0.EnsureValidToken()))
		router.Use(common.Auth0IdMiddleware())
	} else {
		router.Use(common.TestAuth0IdMiddleware())
	}

	// Set up our user-fetching middleware
	getUser := a.genGetUserFn()
	userMiddleware := common.UserMiddleware(getUser, true)
	userMiddlewareLenient := common.UserMiddleware(getUser, false)

	v1 := router.Group("/api/v1")
	{
		// The two routes that can't take the standard chain below.
		//
		// Creating/upserting the caller's own row runs the lenient user
		// middleware because a first-login caller has no users row to load yet,
		// and it names no user in its path (customCreateOrUpdateUser takes the
		// auth0_id from the token).
		v1.POST("/users", userMiddlewareLenient, a.customCreateOrUpdateUser)
		v1.POST("/users/", userMiddlewareLenient, a.customCreateOrUpdateUser)
		// A problem row belongs to no one and is fetched by its own id, so it
		// needs no users row loaded (the validated JWT is still required).
		v1.GET("/problems/:id", a.getProblem)

		// Everything else shares one chain: resolve the token to a users row,
		// then refuse any request whose path names a different user. Carrying
		// RequireSelf on the group is what makes a route added here self-only by
		// construction instead of by remembering — see self_access.go.
		authed := v1.Group("", userMiddleware, a.RequireSelf())
		{
			authed.GET("/pageload/:auth0_id", a.customGetPageLoadData)
			authed.GET("/play/:user_id", a.customGetPlayData)
			authed.GET("/statistics/:user_id", a.getStatistics)
			user := authed.Group("/users")
			{
				user.POST("/:auth0_id", a.customUpdateUser)
				user.GET("/:auth0_id", a.getUser)
				// Self-service account deletion (Adults section). PIN-gated;
				// purges per-user state, anonymizes the users row, best-effort
				// removes the Auth0 identity.
				user.DELETE("/:auth0_id", a.customDeleteAccount)
			}
			settings := authed.Group("/settings")
			{
				settings.POST("/:user_id", a.customUpdateSettings)
				settings.GET("/:user_id", a.getSettings)
			}
			gamestate := authed.Group("/gamestates")
			{
				gamestate.GET("/:user_id", a.customGetGamestate)
			}
			video := authed.Group("/videos")
			{
				video.POST("", a.customCreateVideo)
				video.POST("/", a.customCreateVideo)
				// No client-facing update: a videos row is shared catalog
				// metadata keyed by nothing but its own id, so an update route
				// lets any caller rewrite what every user who has that video
				// plays. The one legitimate mutation is the server disabling a
				// video it couldn't play (videoManager.Update on
				// ERROR_PLAYING_VIDEO); cmd/check_disabled_videos re-enables.
				video.DELETE("/:id", a.customDeleteVideo)
				video.GET("/:id", a.getVideo)
				video.GET("", a.customListVideo)
				video.GET("/", a.customListVideo)
			}
			playlists := authed.Group("/playlists")
			{
				playlists.GET("", a.customListPlaylists)
				playlists.GET("/", a.customListPlaylists)
				playlists.POST("", a.customAddPlaylist)
				playlists.POST("/", a.customAddPlaylist)
				playlists.GET("/:playlist_id/videos", a.customListPlaylistVideos)
				playlists.DELETE("/:playlist_id", a.customRemovePlaylist)
			}
			event := authed.Group("/events")
			{
				event.GET("/:user_id/:seconds", a.customListEvent)
				event.POST("", a.customCreateEvent)
				event.POST("/", a.customCreateEvent)
			}
			// Operator-only surfaces. Gated by RequireAdmin (after userMiddleware
			// loads the user from the validated token's identity).
			admin := authed.Group("/admin", a.RequireAdmin())
			{
				admin.GET("/whoami", a.adminWhoami)
				admin.GET("/difficulty-calibration", a.adminDifficultyCalibration)
				admin.POST("/difficulty-calibration/recompute", a.adminRecomputeCalibration)
				admin.GET("/bitmap-matrix", a.adminBitmapMatrix)
				admin.POST("/bitmap-matrix/recompute", a.adminRecomputeBitmapMatrix)
				admin.GET("/bitmap-matrix/cell", a.adminBitmapMatrixCell)
			}
		}
	}
	return router
}

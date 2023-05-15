// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	gin_adapter "github.com/gwatts/gin-adapter"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/auth0"
)

var CREATE_TABLES_SQL = []string{
	CreateUserTableSQL,
	CreateVideoTableSQL,
	CreateProblemTableSQL,
	CreateSettingsTableSQL,
	CreateGamestateTableSQL,
	CreateEventTableSQL,
}

type Api struct {
	DB               *sql.DB
	isTest           bool
	userManager      *UserManager
	videoManager     *VideoManager
	problemManager   *ProblemManager
	settingsManager  *SettingsManager
	gamestateManager *GamestateManager
	eventManager     *EventManager
}

func NewApi(db *sql.DB) (*Api, error) {
	for _, sql := range CREATE_TABLES_SQL {
		_, err := db.Exec(sql)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				msg := "Not creating table"
				glog.Infof("%s: %v", msg, err)
				continue
			}
			return nil, err
		}
	}
	a := &Api{DB: db}
	a.userManager = &UserManager{DB: db}
	a.videoManager = &VideoManager{DB: db}
	a.problemManager = &ProblemManager{DB: db}
	a.settingsManager = &SettingsManager{DB: db}
	a.gamestateManager = &GamestateManager{DB: db}
	a.eventManager = &EventManager{DB: db}
	return a, nil
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
	config.AllowMethods = []string{"GET", "POST", "DELETE"}
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
		user := v1.Group("/users")
		{
			user.POST("", userMiddlewareLenient, a.customCreateOrUpdateUser)
			user.POST("/", userMiddlewareLenient, a.customCreateOrUpdateUser)
			user.POST("/:auth0_id", userMiddleware, a.updateUser)
			user.GET("/:auth0_id", userMiddleware, a.getUser)
		}
		settings := v1.Group("/settings")
		{
			settings.POST("/:user_id", userMiddleware, a.customUpdateSettings)
			settings.GET("/:user_id", userMiddleware, a.getSettings)
		}
		gamestate := v1.Group("/gamestates")
		{
			gamestate.GET("/:user_id", userMiddleware, a.getGamestate)
		}
		video := v1.Group("/videos")
		{
			video.POST("", userMiddleware, a.createVideo)
			video.POST("/", userMiddleware, a.createVideo)
			video.POST("/:id", userMiddleware, a.updateVideo)
			video.DELETE("/:id", userMiddleware, a.customDeleteVideo)
			video.GET("/num_enabled", userMiddleware, a.customGetNumEnabledVideos)
			video.GET("/:id", userMiddleware, a.getVideo)
			video.GET("", userMiddleware, a.customListVideo)
			video.GET("/", userMiddleware, a.customListVideo)
		}
		problem := v1.Group("/problems")
		{
			problem.GET("/:id", a.getProblem)
		}
		event := v1.Group("/events")
		{
			event.GET("/:user_id/:seconds", userMiddleware, a.customListEvent)
			event.POST("", userMiddleware, a.customCreateEvent)
			event.POST("/", userMiddleware, a.customCreateEvent)
		}
	}
	return router
}

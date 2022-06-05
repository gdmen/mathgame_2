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
	CreateGamestateTableSQL,
	CreateEventTableSQL,
}

type Error struct {
	Message string `json:"message" form:"message"`
}

func GetError(message string) map[string]interface{} {
	return gin.H{"message": message}
}

type Api struct {
	DB               *sql.DB
	isTest           bool
	userManager      *UserManager
	videoManager     *VideoManager
	problemManager   *ProblemManager
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
	a.gamestateManager = &GamestateManager{DB: db}
	a.eventManager = &EventManager{DB: db}
	return a, nil
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

	// Use our auth0 jwt middleware
	if !a.isTest {
		router.Use(gin_adapter.Wrap(auth0.EnsureValidToken()))
	}

	v1 := router.Group("/api/v1")
	{
		user := v1.Group("/users")
		{
			user.POST("", common.RequestIdMiddleware(), a.createUser)
			user.POST("/", common.RequestIdMiddleware(), a.createUser)
			user.POST("/:auth0_id", common.RequestIdMiddleware(), a.updateUser)
			user.GET("/:auth0_id", common.RequestIdMiddleware(), a.getUser)
		}
		video := v1.Group("/videos")
		{
			video.POST("", common.RequestIdMiddleware(), a.createVideo)
			video.POST("/", common.RequestIdMiddleware(), a.createVideo)
			video.POST("/:id", common.RequestIdMiddleware(), a.updateVideo)
			video.DELETE("/:id", common.RequestIdMiddleware(), a.deleteVideo)
			video.GET("/:id", common.RequestIdMiddleware(), a.getVideo)
			video.GET("", common.RequestIdMiddleware(), a.listVideo)
			video.GET("/", common.RequestIdMiddleware(), a.listVideo)
		}
		problem := v1.Group("/problems")
		{
			problem.GET("/:id", common.RequestIdMiddleware(), a.getProblem)
		}
		event := v1.Group("/events")
		{
			event.POST("", common.RequestIdMiddleware(), a.createEvent)
			event.POST("/", common.RequestIdMiddleware(), a.createEvent)
		}
	}
	return router
}

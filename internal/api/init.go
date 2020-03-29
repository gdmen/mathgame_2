// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/internal/api"

import (
	"database/sql"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/internal/common"
)

var CREATE_TABLES_SQL = []string{
	CreateVideoTableSQL,
}

type Api struct {
	DB *sql.DB
}

func NewApi(db *sql.DB) (*Api, error) {
	for _, sql := range CREATE_TABLES_SQL {
		_, err := db.Exec(sql)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return nil, err
		}
	}
	return &Api{DB: db}, nil
}

func (a *Api) GetRouter() *gin.Engine {
	router := gin.Default()
	// Allow all origins, methods
	router.Use(cors.Default())

	v1 := router.Group("/api/v1")
	{
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
	}
	return router
}

// Package api contains api routes, handlers, and models
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

// ProgressResponse is the JSON response for GET /api/v1/progress/:user_id
type ProgressResponse struct {
	TotalProblemsSolved int64 `json:"total_problems_solved"`
	TotalWorkMinutes    int64 `json:"total_work_minutes"`
	TotalVideoMinutes   int64 `json:"total_video_minutes"`
}

func (a *Api) getProgress(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	user := GetUserFromContext(c)

	var params struct {
		UserId uint32 `uri:"user_id"`
	}
	if BindModelFromURI(logPrefix, c, &params) != nil {
		return
	}
	if params.UserId != user.Id {
		c.JSON(http.StatusForbidden, common.GetError("Forbidden"))
		return
	}

	// Single query: one index seek on (user_id, event_type), conditional aggregation for all three metrics.
	// Value is stored in milliseconds; sum first, then DIV 60000 to get minutes. SIGNED cast so negative values (measurement inaccuracies) don't wrap.
	const msPerMinute = 60000
	var totalProblems, totalWorkMinutes, totalVideoMinutes int64
	err := a.DB.QueryRow(`
		SELECT
			COUNT(CASE WHEN event_type = ? THEN 1 END),
			(GREATEST(COALESCE(SUM(CASE WHEN event_type = ? THEN CAST(value AS SIGNED) END), 0), 0)) DIV ?,
			(GREATEST(COALESCE(SUM(CASE WHEN event_type = ? THEN CAST(value AS SIGNED) END), 0), 0)) DIV ?
		FROM events
		WHERE user_id = ? AND event_type IN (?, ?, ?)`,
		SOLVED_PROBLEM, WORKING_ON_PROBLEM, msPerMinute, WATCHING_VIDEO, msPerMinute,
		user.Id, SOLVED_PROBLEM, WORKING_ON_PROBLEM, WATCHING_VIDEO,
	).Scan(&totalProblems, &totalWorkMinutes, &totalVideoMinutes)
	if err != nil {
		glog.Errorf("%s progress totals: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not get progress"))
		return
	}

	resp := ProgressResponse{
		TotalProblemsSolved: totalProblems,
		TotalWorkMinutes:    totalWorkMinutes,
		TotalVideoMinutes:   totalVideoMinutes,
	}
	HandleMngrRespWriteCtx(logPrefix, c, http.StatusOK, "", nil, resp)
}

// Package api contains api routes, handlers, and models
package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

// StatisticsResponse is the JSON response for GET /api/v1/statistics/:user_id
type StatisticsResponse struct {
	TotalProblemsSolved int64        `json:"total_problems_solved"`
	TotalWorkMinutes    int64        `json:"total_work_minutes"`
	TotalVideoMinutes   int64        `json:"total_video_minutes"`
	StatsByMonth        []MonthStats `json:"stats_by_month"`
}

// MonthStats holds the same top-level stats for a single month (YYYY-MM).
type MonthStats struct {
	Month               string `json:"month"`
	TotalProblemsSolved int64  `json:"total_problems_solved"`
	TotalWorkMinutes    int64  `json:"total_work_minutes"`
	TotalVideoMinutes   int64  `json:"total_video_minutes"`
}

func (a *Api) getStatistics(c *gin.Context) {
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

	lastEventID, metaExists, err := a.getProgressLastEventID(user.Id)
	if err != nil {
		glog.Errorf("%s get last_event_id: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not get statistics"))
		return
	}

	if !metaExists {
		if err := a.fullProgressBackfill(logPrefix, user.Id); err != nil {
			glog.Errorf("%s full backfill: %v", logPrefix, err)
			c.JSON(http.StatusInternalServerError, common.GetError("Could not get statistics"))
			return
		}
	} else {
		newEvents, err := a.fetchProgressEventsAfter(user.Id, lastEventID)
		if err != nil {
			glog.Errorf("%s fetch new events: %v", logPrefix, err)
			c.JSON(http.StatusInternalServerError, common.GetError("Could not get statistics"))
			return
		}
		if len(newEvents) > 0 {
			if _, err := a.mergeProgressEventsIntoCache(logPrefix, user.Id, newEvents); err != nil {
				glog.Errorf("%s merge events into cache: %v", logPrefix, err)
				c.JSON(http.StatusInternalServerError, common.GetError("Could not get statistics"))
				return
			}
		}
	}

	resp, err := a.readStatisticsFromCache(logPrefix, user.Id)
	if err != nil {
		glog.Errorf("%s read from cache: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not get statistics"))
		return
	}
	HandleMngrRespWriteCtx(logPrefix, c, http.StatusOK, "", nil, resp)
}

type progressEventRow struct {
	id        uint64
	eventType string
	value     string
	timestamp time.Time
}

func (a *Api) getProgressLastEventID(userID uint32) (lastEventID uint64, exists bool, err error) {
	var last uint64
	err = a.DB.QueryRow(`SELECT last_event_id FROM statistics_cache_meta WHERE user_id = ?`, userID).Scan(&last)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return last, true, nil
}

func (a *Api) fetchProgressEventsAfter(userID uint32, afterID uint64) ([]progressEventRow, error) {
	rows, err := a.DB.Query(
		`SELECT id, event_type, value, timestamp FROM events WHERE user_id = ? AND id > ? ORDER BY id`,
		userID, afterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []progressEventRow
	for rows.Next() {
		var r progressEventRow
		if err := rows.Scan(&r.id, &r.eventType, &r.value, &r.timestamp); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (a *Api) fullProgressBackfill(logPrefix string, userID uint32) error {
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
		userID, SOLVED_PROBLEM, WORKING_ON_PROBLEM, WATCHING_VIDEO,
	).Scan(&totalProblems, &totalWorkMinutes, &totalVideoMinutes)
	if err != nil {
		return err
	}

	_, err = a.DB.Exec(`
		INSERT INTO statistics_totals (user_id, total_problems_solved, total_work_minutes, total_video_minutes)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			total_problems_solved = VALUES(total_problems_solved),
			total_work_minutes = VALUES(total_work_minutes),
			total_video_minutes = VALUES(total_video_minutes)`,
		userID, totalProblems, totalWorkMinutes, totalVideoMinutes,
	)
	if err != nil {
		return err
	}

	monthRows, err := a.DB.Query(`
		SELECT
			DATE_FORMAT(timestamp, '%Y-%m') AS month,
			COUNT(CASE WHEN event_type = ? THEN 1 END),
			(GREATEST(COALESCE(SUM(CASE WHEN event_type = ? THEN CAST(value AS SIGNED) END), 0), 0)) DIV ?,
			(GREATEST(COALESCE(SUM(CASE WHEN event_type = ? THEN CAST(value AS SIGNED) END), 0), 0)) DIV ?
		FROM events
		WHERE user_id = ? AND event_type IN (?, ?, ?)
		GROUP BY DATE_FORMAT(timestamp, '%Y-%m')`,
		SOLVED_PROBLEM, WORKING_ON_PROBLEM, msPerMinute, WATCHING_VIDEO, msPerMinute,
		userID, SOLVED_PROBLEM, WORKING_ON_PROBLEM, WATCHING_VIDEO,
	)
	if err != nil {
		return err
	}
	defer monthRows.Close()
	for monthRows.Next() {
		var month string
		var solved int64
		var workMin, videoMin int64
		if err := monthRows.Scan(&month, &solved, &workMin, &videoMin); err != nil {
			return err
		}
		_, err = a.DB.Exec(`
			INSERT INTO statistics_monthly (user_id, month, total_problems_solved, total_work_minutes, total_video_minutes)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				total_problems_solved = VALUES(total_problems_solved),
				total_work_minutes = VALUES(total_work_minutes),
				total_video_minutes = VALUES(total_video_minutes)`,
			userID, month, solved, workMin, videoMin,
		)
		if err != nil {
			return err
		}
	}
	if err := monthRows.Err(); err != nil {
		return err
	}

	var maxID uint64
	err = a.DB.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM events WHERE user_id = ?`, userID).Scan(&maxID)
	if err != nil {
		return err
	}
	_, err = a.DB.Exec(`
		INSERT INTO statistics_cache_meta (user_id, last_event_id) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE last_event_id = VALUES(last_event_id)`,
		userID, maxID,
	)
	return err
}

func (a *Api) mergeProgressEventsIntoCache(logPrefix string, userID uint32, events []progressEventRow) (uint64, error) {
	if len(events) == 0 {
		return 0, nil
	}
	const msPerMinute = 60000
	var totalDelta int64
	var workDelta, videoDelta int64
	monthDeltas := make(map[string]struct {
		solved   int64
		workMin  int64
		videoMin int64
	})
	for _, e := range events {
		switch e.eventType {
		case SOLVED_PROBLEM:
			totalDelta++
		case WORKING_ON_PROBLEM:
			v, _ := strconv.ParseInt(e.value, 10, 64)
			if v > 0 {
				workDelta += v
			}
		case WATCHING_VIDEO:
			v, _ := strconv.ParseInt(e.value, 10, 64)
			if v > 0 {
				videoDelta += v
			}
		}
		if e.eventType == SOLVED_PROBLEM || e.eventType == WORKING_ON_PROBLEM || e.eventType == WATCHING_VIDEO {
			month := e.timestamp.Format("2006-01")
			d := monthDeltas[month]
			switch e.eventType {
			case SOLVED_PROBLEM:
				d.solved++
			case WORKING_ON_PROBLEM:
				v, _ := strconv.ParseInt(e.value, 10, 64)
				if v > 0 {
					d.workMin += v / msPerMinute
				}
			case WATCHING_VIDEO:
				v, _ := strconv.ParseInt(e.value, 10, 64)
				if v > 0 {
					d.videoMin += v / msPerMinute
				}
			}
			monthDeltas[month] = d
		}
	}
	workMinDelta := workDelta / msPerMinute
	videoMinDelta := videoDelta / msPerMinute

	_, err := a.DB.Exec(`
		INSERT INTO statistics_totals (user_id, total_problems_solved, total_work_minutes, total_video_minutes)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			total_problems_solved = total_problems_solved + VALUES(total_problems_solved),
			total_work_minutes = total_work_minutes + VALUES(total_work_minutes),
			total_video_minutes = total_video_minutes + VALUES(total_video_minutes)`,
		userID, totalDelta, workMinDelta, videoMinDelta,
	)
	if err != nil {
		return 0, err
	}

	for month, d := range monthDeltas {
		_, err = a.DB.Exec(`
			INSERT INTO statistics_monthly (user_id, month, total_problems_solved, total_work_minutes, total_video_minutes)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				total_problems_solved = total_problems_solved + VALUES(total_problems_solved),
				total_work_minutes = total_work_minutes + VALUES(total_work_minutes),
				total_video_minutes = total_video_minutes + VALUES(total_video_minutes)`,
			userID, month, d.solved, d.workMin, d.videoMin,
		)
		if err != nil {
			return 0, err
		}
	}

	var maxID uint64
	for _, e := range events {
		if e.id > maxID {
			maxID = e.id
		}
	}
	_, err = a.DB.Exec(`
		INSERT INTO statistics_cache_meta (user_id, last_event_id) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE last_event_id = VALUES(last_event_id)`,
		userID, maxID,
	)
	if err != nil {
		return 0, err
	}
	return maxID, nil
}

func (a *Api) readStatisticsFromCache(logPrefix string, userID uint32) (StatisticsResponse, error) {
	var resp StatisticsResponse
	err := a.DB.QueryRow(`
		SELECT total_problems_solved, total_work_minutes, total_video_minutes
		FROM statistics_totals WHERE user_id = ?`, userID,
	).Scan(&resp.TotalProblemsSolved, &resp.TotalWorkMinutes, &resp.TotalVideoMinutes)
	if err == sql.ErrNoRows {
		return StatisticsResponse{StatsByMonth: []MonthStats{}}, nil
	}
	if err != nil {
		return resp, err
	}

	rows, err := a.DB.Query(`
		SELECT month, total_problems_solved, total_work_minutes, total_video_minutes
		FROM statistics_monthly WHERE user_id = ? ORDER BY month DESC`, userID,
	)
	if err != nil {
		return resp, err
	}
	defer rows.Close()
	for rows.Next() {
		var m MonthStats
		if err := rows.Scan(&m.Month, &m.TotalProblemsSolved, &m.TotalWorkMinutes, &m.TotalVideoMinutes); err != nil {
			return resp, err
		}
		resp.StatsByMonth = append(resp.StatsByMonth, m)
	}
	if err := rows.Err(); err != nil {
		return resp, err
	}
	return resp, nil
}

// computeStatsByMonth returns totals grouped by month (YYYY-MM), ordered by month descending.
func (a *Api) computeStatsByMonth(logPrefix string, userID uint32) ([]MonthStats, error) {
	const msPerMinute = 60000
	rows, err := a.DB.Query(`
		SELECT
			DATE_FORMAT(timestamp, '%Y-%m') AS month,
			COUNT(CASE WHEN event_type = ? THEN 1 END),
			(GREATEST(COALESCE(SUM(CASE WHEN event_type = ? THEN CAST(value AS SIGNED) END), 0), 0)) DIV ?,
			(GREATEST(COALESCE(SUM(CASE WHEN event_type = ? THEN CAST(value AS SIGNED) END), 0), 0)) DIV ?
		FROM events
		WHERE user_id = ? AND event_type IN (?, ?, ?)
		GROUP BY DATE_FORMAT(timestamp, '%Y-%m')
		ORDER BY month DESC`,
		SOLVED_PROBLEM, WORKING_ON_PROBLEM, msPerMinute, WATCHING_VIDEO, msPerMinute,
		userID, SOLVED_PROBLEM, WORKING_ON_PROBLEM, WATCHING_VIDEO,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MonthStats
	for rows.Next() {
		var m MonthStats
		if err := rows.Scan(&m.Month, &m.TotalProblemsSolved, &m.TotalWorkMinutes, &m.TotalVideoMinutes); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

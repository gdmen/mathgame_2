// Package api contains api routes, handlers, and models
package api

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

// StatisticsResponse is the JSON response for GET /api/v1/statistics/:user_id
type StatisticsResponse struct {
	TotalProblemsSolved int64            `json:"total_problems_solved"`
	TotalWorkMinutes    int64            `json:"total_work_minutes"`
	TotalVideoMinutes   int64            `json:"total_video_minutes"`
	StatsByMonth        []MonthStats     `json:"stats_by_month"`
	HardestProblems     []HardestProblem `json:"hardest_problems"`
}

// MonthStats holds the same top-level stats for a single month (YYYY-MM).
type MonthStats struct {
	Month               string `json:"month"`
	TotalProblemsSolved int64  `json:"total_problems_solved"`
	TotalWorkMinutes    int64  `json:"total_work_minutes"`
	TotalVideoMinutes   int64  `json:"total_video_minutes"`
}

// HardestProblem is one of the top 20 problems by average time to solve.
type HardestProblem struct {
	ProblemId           uint32  `json:"problem_id"`
	Expression          string  `json:"expression"`
	Answer              string  `json:"answer"`
	AvgTimeToSolveMs    int64   `json:"avg_time_to_solve_ms"`
	AvgAttemptsPerSolve float64 `json:"avg_attempts_per_solve"`
	TimesSeen           int     `json:"times_seen"`
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
		glog.Errorf("%s statistics totals: %v", logPrefix, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Could not get statistics"))
		return
	}

	// Stats by month: same metrics grouped by YYYY-MM, recent first.
	statsByMonth, err := a.computeStatsByMonth(logPrefix, user.Id)
	if err != nil {
		glog.Errorf("%s stats by month: %v", logPrefix, err)
		statsByMonth = []MonthStats{}
	}

	// 20 hardest problems: segment events into solve sessions (SELECTED -> ... -> SOLVED), aggregate by problem_id, sort by avg time desc.
	hardest := a.computeHardestProblems(logPrefix, user.Id)
	if hardest == nil {
		hardest = []HardestProblem{}
	}
	// Enrich with problem expression and answer from problems table.
	for i := range hardest {
		p, _, _, err := a.problemManager.Get(hardest[i].ProblemId)
		if err == nil && p != nil {
			hardest[i].Expression = p.Expression
			hardest[i].Answer = p.Answer
		}
	}

	resp := StatisticsResponse{
		TotalProblemsSolved: totalProblems,
		TotalWorkMinutes:    totalWorkMinutes,
		TotalVideoMinutes:   totalVideoMinutes,
		StatsByMonth:        statsByMonth,
		HardestProblems:     hardest,
	}
	HandleMngrRespWriteCtx(logPrefix, c, http.StatusOK, "", nil, resp)
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

// computeHardestProblems returns up to 20 problems with highest avg time to solve (ms). Each solve session is SELECTED_PROBLEM .. WORKING_ON_PROBLEM / ANSWERED_PROBLEM .. SOLVED_PROBLEM.
func (a *Api) computeHardestProblems(logPrefix string, userID uint32) []HardestProblem {
	rows, err := a.DB.Query(
		`SELECT event_type, value FROM events WHERE user_id = ? AND event_type IN (?, ?, ?, ?) ORDER BY id`,
		userID, SELECTED_PROBLEM, WORKING_ON_PROBLEM, ANSWERED_PROBLEM, SOLVED_PROBLEM,
	)
	if err != nil {
		glog.Errorf("%s hardest problems query: %v", logPrefix, err)
		return nil
	}
	defer rows.Close()

	var segments []struct {
		problemID uint32
		timeMs    int64
		attempts  int
	}

	var inSegment bool
	var segTimeMs int64
	var segAttempts int

	for rows.Next() {
		var eventType, value string
		if err := rows.Scan(&eventType, &value); err != nil {
			glog.Errorf("%s scan event: %v", logPrefix, err)
			return nil
		}
		switch eventType {
		case SELECTED_PROBLEM:
			inSegment = true
			segTimeMs = 0
			segAttempts = 0
		case WORKING_ON_PROBLEM:
			if inSegment {
				v, _ := strconv.ParseInt(value, 10, 64)
				if v > 0 {
					segTimeMs += v
				}
			}
		case ANSWERED_PROBLEM:
			if inSegment {
				segAttempts++
			}
		case SOLVED_PROBLEM:
			if inSegment {
				pid, err := strconv.ParseUint(value, 10, 32)
				if err == nil {
					segments = append(segments, struct {
						problemID uint32
						timeMs    int64
						attempts  int
					}{uint32(pid), segTimeMs, segAttempts + 1})
				}
			}
			inSegment = false
		}
	}
	if err := rows.Err(); err != nil {
		glog.Errorf("%s hardest problems rows: %v", logPrefix, err)
		return nil
	}

	// Aggregate by problem_id: sum time, sum attempts, count
	type agg struct {
		totalTimeMs   int64
		totalAttempts int
		count         int
	}
	byProblem := make(map[uint32]*agg)
	for _, s := range segments {
		if byProblem[s.problemID] == nil {
			byProblem[s.problemID] = &agg{}
		}
		byProblem[s.problemID].totalTimeMs += s.timeMs
		byProblem[s.problemID].totalAttempts += s.attempts
		byProblem[s.problemID].count++
	}

	var result []HardestProblem
	for pid, ag := range byProblem {
		if ag.count == 0 {
			continue
		}
		avgTime := ag.totalTimeMs / int64(ag.count)
		avgAttempts := float64(ag.totalAttempts) / float64(ag.count)
		result = append(result, HardestProblem{
			ProblemId:           pid,
			AvgTimeToSolveMs:    avgTime,
			AvgAttemptsPerSolve: avgAttempts,
			TimesSeen:           ag.count,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].AvgTimeToSolveMs > result[j].AvgTimeToSolveMs
	})
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

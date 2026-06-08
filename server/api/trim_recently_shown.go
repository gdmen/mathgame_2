package api

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/golang/glog"
)

// TrimRecentlyShownProblems caps each user's row count in
// recently_shown_problems to recentlyShownProblemsTrimSize. Two-step pattern
// (planner SELECT + per-user DELETEs) avoids a single correlated DELETE that
// would re-evaluate its subquery per scanned row.
//
// dryRun: if true, returns the plan without writing. Returns (usersTrimmed,
// rowsDeleted, error). Failures on any single user's delete are logged but
// don't abort the whole pass - the next run will retry them.
func TrimRecentlyShownProblems(db *sql.DB, dryRun bool) (int, int, error) {
	cutoffs, err := planRecentlyShownTrim(db)
	if err != nil {
		return 0, 0, fmt.Errorf("plan: %w", err)
	}
	if dryRun {
		return len(cutoffs), 0, nil
	}
	totalDeleted := 0
	usersTrimmed := 0
	for _, cu := range cutoffs {
		// Per-user delete: only touches that user's recently_shown_problems
		// rows, contends only with that user's in-flight upserts (very short).
		res, err := db.Exec(
			`DELETE FROM recently_shown_problems
			 WHERE user_id = ? AND shown_at < ?`,
			cu.userID, cu.cutoff,
		)
		if err != nil {
			glog.Errorf("trim recently_shown_problems user=%d: %v", cu.userID, err)
			continue
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			usersTrimmed++
			totalDeleted += int(n)
		}
	}
	return usersTrimmed, totalDeleted, nil
}

type recentlyShownCutoff struct {
	userID uint32
	cutoff time.Time // shown_at of the (cap+1)-th most-recent row for this user
}

func planRecentlyShownTrim(db *sql.DB) ([]recentlyShownCutoff, error) {
	// For each user, find the cutoff timestamp: the shown_at of the
	// (recentlyShownProblemsTrimSize+1)-th most recent row. Users with
	// fewer rows than the cap don't have a (cap+1)-th row and so won't
	// appear in this result - they need no trim. Uses MySQL session
	// variables to rank by recency within each user_id (5.7-compatible;
	// no window functions).
	sql := `
		SELECT user_id, shown_at
		FROM (
		    SELECT user_id, shown_at,
		           @rank := IF(@prev_user = user_id, @rank + 1, 1) AS rnk,
		           @prev_user := user_id
		    FROM recently_shown_problems, (SELECT @rank := 0, @prev_user := NULL) init
		    ORDER BY user_id, shown_at DESC
		) ranked
		WHERE rnk = ?`
	rows, err := db.Query(sql, recentlyShownProblemsTrimSize+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []recentlyShownCutoff
	for rows.Next() {
		var c recentlyShownCutoff
		if err := rows.Scan(&c.userID, &c.cutoff); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

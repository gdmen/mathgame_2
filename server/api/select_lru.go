package api

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"
)

// pickWithRecencyBias picks a candidate id, biased toward least-recently-shown.
// lruTopFrac, recentlyShownProblemsTrimSize, etc. live in generate_problems.go.
func (a *Api) pickWithRecencyBias(logPrefix string, userID uint32, candidates []uint32) uint32 {
	if len(candidates) == 0 {
		return 0
	}
	local := append([]uint32(nil), candidates...)
	lastShown, err := a.lastShownAt(userID, local)
	if err != nil {
		glog.Errorf("%s lastShownAt: %v (falling back to uniform random)", logPrefix, err)
		return local[rand.Intn(len(local))]
	}
	sort.SliceStable(local, recencyLess(local, lastShown))
	topN := int(float64(len(local)) * lruTopFrac)
	if topN < 1 {
		topN = 1
	}
	return local[rand.Intn(topN)]
}

// recencyLess orders never-shown first, then oldest-shown to most-recent.
func recencyLess(ids []uint32, lastShown map[uint32]time.Time) func(i, j int) bool {
	return func(i, j int) bool {
		ti, iSeen := lastShown[ids[i]]
		tj, jSeen := lastShown[ids[j]]
		switch {
		case !iSeen && jSeen:
			return true
		case iSeen && !jSeen:
			return false
		case !iSeen && !jSeen:
			return false
		default:
			return ti.Before(tj)
		}
	}
}

// lastShownAt returns the latest shown_at timestamp per id for the user from
// recently_shown_problems. Ids not in the result are treated as "never shown"
// by recencyLess, which is the right semantics: the cache is trimmed to
// recentlyShownProblemsTrimSize per user, so anything evicted is functionally
// forgotten and should re-enter the rotation.
func (a *Api) lastShownAt(userID uint32, ids []uint32) (map[uint32]time.Time, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	// PK lookup on the bounded recently_shown_problems cache: one row per
	// (user_id, problem_id), so this is at most len(ids) rows, indexed.
	sql := fmt.Sprintf(
		`SELECT problem_id, shown_at FROM recently_shown_problems
		 WHERE user_id=? AND problem_id IN (%s)`, placeholders)
	args := make([]interface{}, 0, 1+len(ids))
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := a.DB.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint32]time.Time, len(ids))
	for rows.Next() {
		var id uint32
		var ts time.Time
		if err := rows.Scan(&id, &ts); err != nil {
			return nil, err
		}
		out[id] = ts
	}
	return out, rows.Err()
}

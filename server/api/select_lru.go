package api

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

// lruTopFrac is the fraction of recency-sorted candidates we pick uniformly from.
const lruTopFrac = 0.20

// pickWithRecencyBias picks a candidate id, biased toward least-recently-shown.
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

// lastShownAt returns the latest SELECTED_PROBLEM timestamp per id for the user.
func (a *Api) lastShownAt(userID uint32, ids []uint32) (map[uint32]time.Time, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	sql := fmt.Sprintf(
		`SELECT value, MAX(timestamp) FROM events
		 WHERE user_id=? AND event_type=? AND value IN (%s)
		 GROUP BY value`, placeholders)
	args := make([]interface{}, 0, 2+len(ids))
	args = append(args, userID, SELECTED_PROBLEM)
	for _, id := range ids {
		args = append(args, strconv.FormatUint(uint64(id), 10))
	}
	rows, err := a.DB.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint32]time.Time, len(ids))
	for rows.Next() {
		var valStr string
		var ts time.Time
		if err := rows.Scan(&valStr, &ts); err != nil {
			return nil, err
		}
		id, perr := strconv.ParseUint(valStr, 10, 32)
		if perr != nil {
			continue
		}
		out[uint32(id)] = ts
	}
	return out, rows.Err()
}

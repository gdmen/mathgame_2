// Package api: pool-supply weighting for topic selection.
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here REQUIRE updating that doc in the same PR.
//
// This is the SUPPLY-side signal in the serving lottery
// (chooseWeightedTopic): topics whose pool is thin get a proportionally
// larger weight, so hard-to-generate bits (WORD, DECIMALS, PEMDAS,
// PERCENTAGES) keep appearing without starving everything else - and a
// picked-but-thin topic triggers background generation, which fills its
// pool. The DEMAND-side signal (per-kid skill weighting) lives in
// topic_stats.go.
package api

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

const (
	// poolCountsCacheTTL bounds how stale the per-bit pool counts may be.
	// Counts move slowly (generation adds ~20 rows at a time to a 300K-row
	// pool), so minutes of staleness are harmless.
	poolCountsCacheTTL = 5 * time.Minute
	// thinPoolBoostMax caps the rarity multiplier. Tunable: at 4.0 a
	// nearly-empty topic gets at most 4x the lottery weight of an average
	// one - enough to keep rare bits in rotation without flooding the kid
	// with their thinnest topic.
	thinPoolBoostMax = 4.0
)

var poolCountsCache struct {
	sync.Mutex
	counts    map[uint64]int64 // per-bit problem counts (enabled rows only)
	fetchedAt time.Time
}

// poolCountsByBit returns per-bit pool counts (disabled=0 rows), cached for
// poolCountsCacheTTL. Fail-tolerant: on query error it returns nil, which
// disables supply weighting for that selection rather than failing it.
func (a *Api) poolCountsByBit(logPrefix string) map[uint64]int64 {
	poolCountsCache.Lock()
	defer poolCountsCache.Unlock()
	if poolCountsCache.counts != nil && time.Since(poolCountsCache.fetchedAt) < poolCountsCacheTTL {
		return poolCountsCache.counts
	}

	// Distinct bitmap values are few (hundreds at most); aggregate per bit
	// in Go rather than running 16 COUNT(*) scans.
	rows, err := a.DB.Query(
		`SELECT problem_type_bitmap, COUNT(*) FROM problems WHERE disabled = 0 GROUP BY problem_type_bitmap`)
	if err != nil {
		glog.Warningf("%s poolCountsByBit: %v (pool-supply weighting disabled this round)", logPrefix, err)
		return poolCountsCache.counts // possibly stale or nil; both fine
	}
	defer rows.Close()

	counts := map[uint64]int64{}
	for rows.Next() {
		var bitmap uint64
		var n int64
		if err := rows.Scan(&bitmap, &n); err != nil {
			glog.Warningf("%s poolCountsByBit scan: %v", logPrefix, err)
			return poolCountsCache.counts
		}
		for i := 0; i < 64; i++ {
			bit := uint64(1) << i
			if bitmap&bit != 0 {
				counts[bit] += n
			}
		}
	}
	if err := rows.Err(); err != nil {
		glog.Warningf("%s poolCountsByBit iter: %v", logPrefix, err)
		return poolCountsCache.counts
	}
	poolCountsCache.counts = counts
	poolCountsCache.fetchedAt = time.Now()
	return counts
}

// thinPoolBoost converts a topic's pool count into a serving-lottery
// multiplier: average-pool topics get ~1.0, thin pools approach
// thinPoolBoostMax. avgCount is the mean pool count across the candidate
// topics.
func thinPoolBoost(count, avgCount int64) float64 {
	if avgCount <= 0 || count >= avgCount {
		return 1.0
	}
	if count <= 0 {
		return thinPoolBoostMax
	}
	boost := float64(avgCount) / float64(count)
	if boost > thinPoolBoostMax {
		boost = thinPoolBoostMax
	}
	return boost
}

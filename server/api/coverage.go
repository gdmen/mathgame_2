// Package api: pool-coverage weighting for topic selection.
//
// Part of the problem-generation system - documented in docs/problem-generation.md
// (lands with issue #225 PR3). Behavior changes here REQUIRE updating that doc
// in the same PR.
//
// This is the actual replacement for the old WORD wart (selection force-
// serving word problems whenever WORD was enabled): instead of hard-forcing
// one bit, topics whose pool is thin get a proportionally larger lottery
// weight, so hard-to-generate bits (WORD, and later DECIMALS/PEMDAS/
// PERCENTAGES, #227) keep appearing without starving everything else.
package api

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

const (
	// coverageCacheTTL bounds how stale the per-bit pool counts may be.
	// Counts move slowly (generation adds ~20 rows at a time to a 300K-row
	// pool), so minutes of staleness are harmless.
	coverageCacheTTL = 5 * time.Minute
	// coverageBoostMax caps the rarity multiplier. Tunable: at 4.0 a
	// nearly-empty topic gets at most 4x the lottery weight of an average
	// one - enough to keep rare bits in rotation without flooding the kid
	// with their thinnest topic.
	coverageBoostMax = 4.0
)

var coverageCache struct {
	sync.Mutex
	counts    map[uint64]int64 // per-bit problem counts (enabled rows only)
	fetchedAt time.Time
}

// poolCountsByBit returns per-bit pool counts (disabled=0 rows), cached for
// coverageCacheTTL. Fail-tolerant: on query error it returns nil, which
// disables coverage weighting for that selection rather than failing it.
func (a *Api) poolCountsByBit(logPrefix string) map[uint64]int64 {
	coverageCache.Lock()
	defer coverageCache.Unlock()
	if coverageCache.counts != nil && time.Since(coverageCache.fetchedAt) < coverageCacheTTL {
		return coverageCache.counts
	}

	// Distinct bitmap values are few (hundreds at most); aggregate per bit
	// in Go rather than running 16 COUNT(*) scans.
	rows, err := a.DB.Query(
		`SELECT problem_type_bitmap, COUNT(*) FROM problems WHERE disabled = 0 GROUP BY problem_type_bitmap`)
	if err != nil {
		glog.Warningf("%s poolCountsByBit: %v (coverage weighting disabled this round)", logPrefix, err)
		return coverageCache.counts // possibly stale or nil; both fine
	}
	defer rows.Close()

	counts := map[uint64]int64{}
	for rows.Next() {
		var bitmap uint64
		var n int64
		if err := rows.Scan(&bitmap, &n); err != nil {
			glog.Warningf("%s poolCountsByBit scan: %v", logPrefix, err)
			return coverageCache.counts
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
		return coverageCache.counts
	}
	coverageCache.counts = counts
	coverageCache.fetchedAt = time.Now()
	return counts
}

// coverageBoost converts a topic's pool count into a lottery multiplier:
// average-pool topics get ~1.0, thin pools approach coverageBoostMax.
// avgCount is the mean pool count across the candidate topics.
func coverageBoost(count, avgCount int64) float64 {
	if avgCount <= 0 || count >= avgCount {
		return 1.0
	}
	if count <= 0 {
		return coverageBoostMax
	}
	boost := float64(avgCount) / float64(count)
	if boost > coverageBoostMax {
		boost = coverageBoostMax
	}
	return boost
}

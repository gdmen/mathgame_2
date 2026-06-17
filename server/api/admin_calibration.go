// Package api: the admin difficulty-calibration endpoint.
//
// A read-only JSON endpoint that samples real problems per difficulty bucket
// and returns the ComputeProblemDifficulty factor breakdown for each, so the
// hand-tuned formula constants (server/api/difficulty.go) can be eyeballed
// against the actual prod pool. The web app renders it (web/src/admin_calibration.js).
// Registered under /api/v1/admin behind RequireAdmin. No writes.
//
// The data is gathered in two passes: a GROUP BY for per-bucket live/disabled
// and per-generator counts, then a window-function query for the sample
// problems.
package api

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

// NameCount is a label with its occurrence count (bit frequencies).
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type CalibrationProblem struct {
	Expression string              `json:"expression"`
	Difficulty float64             `json:"difficulty"`
	Bits       []string            `json:"bits"`
	Breakdown  DifficultyBreakdown `json:"breakdown"`
}

// CalibrationGenGroup is one generator version's presence in a bucket: how many
// live problems it has there, and one example of each distinct problem-type
// bitmap it produced.
type CalibrationGenGroup struct {
	Generator string               `json:"generator"`
	LiveCount int                  `json:"live_count"`
	Problems  []CalibrationProblem `json:"problems"`
}

type CalibrationBucket struct {
	Label         string                `json:"label"`
	LiveCount     int                   `json:"live_count"`
	DisabledCount int                   `json:"disabled_count"`
	Generators    []CalibrationGenGroup `json:"generators"`
	DominantBits  []NameCount           `json:"dominant_bits"`
}

type CalibrationData struct {
	Buckets []CalibrationBucket `json:"buckets"`
}

// calibBucketExpr maps a row's difficulty to its bucket key: round-to-nearest,
// so a row falls in the half-open bucket [center-0.5, center+0.5). Shared by
// both passes.
const calibBucketExpr = "CAST(FLOOR(difficulty + 0.5) AS SIGNED)"

// adminDifficultyCalibration samples the live pool into difficulty buckets and
// returns, per bucket and per generator version present, one example of EACH
// distinct problem-type bitmap that generator produced in the bucket (with the
// generator's live count and each example's factor breakdown), as JSON.
func (a *Api) adminDifficultyCalibration(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)

	// Pass 1: live/disabled counts and per-generator live counts for every
	// bucket, in one GROUP BY scan.
	type aggBucket struct {
		live, disabled int
		liveByGen      map[string]int
	}
	agg := map[int]*aggBucket{}
	maxBucket := 0

	rows, err := a.DB.Query(
		"SELECT " + calibBucketExpr + " AS bucket, disabled, generator, COUNT(*) " +
			"FROM problems GROUP BY bucket, disabled, generator")
	if err != nil {
		glog.Errorf("%s calibration counts: %v", logPrefix, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, common.GetError("calibration query failed"))
		return
	}
	for rows.Next() {
		var bucket, disabled, count int
		var generator string
		if rows.Scan(&bucket, &disabled, &generator, &count) != nil {
			continue
		}
		b := agg[bucket]
		if b == nil {
			b = &aggBucket{liveByGen: map[string]int{}}
			agg[bucket] = b
		}
		if disabled == 0 {
			b.live += count
			b.liveByGen[generator] += count
		} else {
			b.disabled += count
		}
		if bucket > maxBucket {
			maxBucket = bucket
		}
	}
	rows.Close()

	// Pass 2: one example per (bucket, generator, problem_type_bitmap) in one
	// window-function scan - rn = 1 over a partition that includes the bitmap,
	// so every distinct problem shape a generator produced in a bucket appears
	// exactly once. ORDER BY id (a content hash, so representative and stable)
	// avoids an ORDER BY RAND() sort.
	type sampleKey struct {
		bucket    int
		generator string
	}
	samples := map[sampleKey][]CalibrationProblem{}
	srows, err := a.DB.Query(
		"WITH live AS (" +
			"SELECT id, expression, difficulty, problem_type_bitmap, generator, " +
			calibBucketExpr + " AS bucket " +
			"FROM problems WHERE disabled = 0) " +
			"SELECT bucket, generator, expression, difficulty, problem_type_bitmap FROM (" +
			"SELECT bucket, generator, expression, difficulty, problem_type_bitmap, " +
			"ROW_NUMBER() OVER (PARTITION BY bucket, generator, problem_type_bitmap ORDER BY id) AS rn FROM live) t " +
			"WHERE rn = 1 ORDER BY bucket, generator, problem_type_bitmap")
	if err != nil {
		glog.Errorf("%s calibration samples: %v", logPrefix, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, common.GetError("calibration query failed"))
		return
	}
	for srows.Next() {
		var bucket int
		var generator, expr string
		var difficulty float64
		var bitmap uint64
		if srows.Scan(&bucket, &generator, &expr, &difficulty, &bitmap) != nil {
			continue
		}
		p := CalibrationProblem{Expression: expr, Difficulty: difficulty}
		p.Bits = ProblemTypeToFeatures(ProblemType(bitmap))
		sort.Strings(p.Bits)
		p.Breakdown = ComputeDifficultyBreakdown(expr)
		k := sampleKey{bucket, generator}
		samples[k] = append(samples[k], p)
	}
	srows.Close()

	// Assemble buckets 1..topCenter from the two maps. topCenter follows the
	// data (no clamp); always show at least the full 1..20 scale.
	topCenter := maxBucket
	if topCenter < 20 {
		topCenter = 20
	}

	data := CalibrationData{Buckets: []CalibrationBucket{}}
	appendBucket := func(key int, label string) {
		bucket := CalibrationBucket{
			Label:        label,
			Generators:   []CalibrationGenGroup{},
			DominantBits: []NameCount{},
		}
		if b := agg[key]; b != nil {
			bucket.LiveCount = b.live
			bucket.DisabledCount = b.disabled
			gens := make([]string, 0, len(b.liveByGen))
			for g := range b.liveByGen {
				gens = append(gens, g)
			}
			sort.Strings(gens) // stable version order across buckets
			bitTally := map[string]int{}
			for _, g := range gens {
				group := CalibrationGenGroup{
					Generator: g,
					LiveCount: b.liveByGen[g],
					Problems:  []CalibrationProblem{},
				}
				for _, p := range samples[sampleKey{key, g}] {
					group.Problems = append(group.Problems, p)
					for _, n := range p.Bits {
						bitTally[n]++
					}
				}
				bucket.Generators = append(bucket.Generators, group)
			}
			bucket.DominantBits = topNameCounts(bitTally, 4)
		}
		data.Buckets = append(data.Buckets, bucket)
	}
	for k := 1; k <= topCenter; k++ {
		appendBucket(k, strconv.Itoa(k))
	}

	c.JSON(http.StatusOK, data)
}

// topNameCounts returns the n most frequent entries in the tally, ties broken
// alphabetically for stable output.
func topNameCounts(tally map[string]int, n int) []NameCount {
	out := make([]NameCount, 0, len(tally))
	for k, v := range tally {
		out = append(out, NameCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

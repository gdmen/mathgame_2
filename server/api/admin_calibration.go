// Package api: the admin difficulty-calibration report.
//
// computeCalibrationReport samples real problems per difficulty bucket with the
// ComputeProblemDifficulty factor breakdown, so the formula constants
// (server/api/difficulty.go) can be eyeballed against the prod pool. It scans
// the whole pool, so the result is cached in the calibration_report table: the
// GET endpoint reads the cache, and the POST recompute endpoint rebuilds it in
// the background. Registered under /api/v1/admin behind RequireAdmin.
// The web app renders it (web/src/admin_calibration.js).
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
)

// NameCount is a label with its occurrence count (bit frequencies).
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type CalibrationProblem struct {
	Expression string                       `json:"expression"`
	Difficulty float64                      `json:"difficulty"`
	Bits       []string                     `json:"bits"`
	Breakdown  mathcore.DifficultyBreakdown `json:"breakdown"`
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

// CalibrationReportResponse is the GET payload: the cached report (nil until
// first computed), when it was computed, and whether a rebuild is in flight.
type CalibrationReportResponse struct {
	Report     *CalibrationData `json:"report"`
	ComputedAt string           `json:"computed_at"`
	Computing  bool             `json:"computing"`
}

// calibrationComputing is true while a background recompute is running. It also
// serves as the dedup guard: only the goroutine that flips it false->true runs.
var calibrationComputing atomic.Bool

// calibBucketExpr maps a row's difficulty to its bucket key: round-to-nearest,
// so a row falls in the half-open bucket [center-0.5, center+0.5).
const calibBucketExpr = "CAST(FLOOR(difficulty + 0.5) AS SIGNED)"

// adminDifficultyCalibration returns the cached calibration report.
func (a *Api) adminDifficultyCalibration(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	resp := CalibrationReportResponse{Computing: calibrationComputing.Load()}

	var blob, computedAt string
	err := a.DB.QueryRow("SELECT report, computed_at FROM calibration_report WHERE id = 1").Scan(&blob, &computedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, resp)
		return
	}
	if err != nil {
		glog.Errorf("%s calibration cache read: %v", logPrefix, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, common.GetError("calibration read failed"))
		return
	}
	var report CalibrationData
	if err := json.Unmarshal([]byte(blob), &report); err != nil {
		glog.Errorf("%s calibration cache unmarshal: %v", logPrefix, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, common.GetError("calibration read failed"))
		return
	}
	resp.Report = &report
	resp.ComputedAt = computedAt
	c.JSON(http.StatusOK, resp)
}

// adminRecomputeCalibration rebuilds the report in the background and stores it,
// returning immediately. A second request while one is running is a no-op.
func (a *Api) adminRecomputeCalibration(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	if !calibrationComputing.CompareAndSwap(false, true) {
		c.JSON(http.StatusOK, gin.H{"computing": true})
		return
	}
	go func() {
		defer calibrationComputing.Store(false)
		defer func() {
			if r := recover(); r != nil {
				glog.Errorf("%s calibration recompute panicked: %v", logPrefix, r)
			}
		}()
		report, err := a.computeCalibrationReport()
		if err != nil {
			glog.Errorf("%s calibration recompute: %v", logPrefix, err)
			return
		}
		blob, err := json.Marshal(report)
		if err != nil {
			glog.Errorf("%s calibration marshal: %v", logPrefix, err)
			return
		}
		if _, err := a.DB.Exec(
			"INSERT INTO calibration_report (id, report, computed_at) VALUES (1, ?, NOW()) "+
				"ON DUPLICATE KEY UPDATE report = VALUES(report), computed_at = VALUES(computed_at)",
			string(blob),
		); err != nil {
			glog.Errorf("%s calibration cache write: %v", logPrefix, err)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"computing": true})
}

// computeCalibrationReport samples the live pool into difficulty buckets: per
// bucket and generator version, one example of each distinct problem-type
// bitmap, with that generator's live count and each example's factor breakdown.
func (a *Api) computeCalibrationReport() (CalibrationData, error) {
	// Pass 1: live/disabled counts and per-generator live counts per bucket.
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
		return CalibrationData{}, err
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

	// Pass 2: one example per (bucket, generator, problem_type_bitmap). Rank on
	// small columns only, then PK-join back for the expression, so the
	// ROW_NUMBER sort never carries the TEXT expression column.
	type sampleKey struct {
		bucket    int
		generator string
	}
	samples := map[sampleKey][]CalibrationProblem{}
	srows, err := a.DB.Query(
		"WITH ranked AS (" +
			"SELECT id, bucket, generator FROM (" +
			"SELECT id, " + calibBucketExpr + " AS bucket, generator, " +
			"ROW_NUMBER() OVER (PARTITION BY " + calibBucketExpr + ", generator, problem_type_bitmap ORDER BY id) AS rn " +
			"FROM problems WHERE disabled = 0) t WHERE rn = 1) " +
			"SELECT r.bucket, r.generator, p.expression, p.symbolic_expression, p.difficulty, p.problem_type_bitmap " +
			"FROM ranked r JOIN problems p ON p.id = r.id " +
			"ORDER BY r.bucket, r.generator")
	if err != nil {
		return CalibrationData{}, err
	}
	for srows.Next() {
		var bucket int
		var generator, expr, symbolic string
		var difficulty float64
		var bitmap uint64
		if srows.Scan(&bucket, &generator, &expr, &symbolic, &difficulty, &bitmap) != nil {
			continue
		}
		p := CalibrationProblem{Expression: expr, Difficulty: difficulty}
		p.Bits = mathcore.ProblemTypeToFeatures(mathcore.ProblemType(bitmap))
		sort.Strings(p.Bits)
		// Scored the same way storage is, so the page matches stored difficulty.
		p.Breakdown = mathcore.ComputeDifficultyBreakdownFor(expr, symbolic)
		k := sampleKey{bucket, generator}
		samples[k] = append(samples[k], p)
	}
	srows.Close()

	// Assemble buckets 1..topCenter; topCenter follows the data, at least 1..20.
	topCenter := maxBucket
	if topCenter < 20 {
		topCenter = 20
	}
	data := CalibrationData{Buckets: []CalibrationBucket{}}
	for k := 1; k <= topCenter; k++ {
		bucket := CalibrationBucket{
			Label:        strconv.Itoa(k),
			Generators:   []CalibrationGenGroup{},
			DominantBits: []NameCount{},
		}
		if b := agg[k]; b != nil {
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
				for _, p := range samples[sampleKey{k, g}] {
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
	return data, nil
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

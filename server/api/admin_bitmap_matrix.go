// Package api: the admin bitmap × difficulty coverage matrix report.
//
// Where admin_calibration.go samples the STORED pool into difficulty buckets,
// this report LIVE-GENERATES one heuristic_2.0 example per (bitmap, difficulty)
// cell across the ENTIRE valid non-WORD bitmap space (mathcore.EnumerateValid
// Bitmaps — exactly 12,960), including cells the pool has never populated, so
// the newest generator can be human-assessed everywhere it could ever be asked
// to build. It overlays a heatmap of current pool usage so the reviewer knows
// where real traffic lives. It is the browser-rendered sibling of
// cmd/compare_generators -mode=matrix.
//
// The whole-space walk is expensive (~150-220k BuildProblem calls), so the
// result is cached, gzipped, in the bitmap_matrix_report table: the GET
// endpoint serves the stored gzip bytes as-is (Content-Encoding: gzip; the
// browser decompresses transparently) and the POST recompute endpoint rebuilds
// it in the background over a worker pool. Registered under /api/v1/admin behind
// RequireAdmin. The web app renders it (web/src/admin_bitmap_matrix.js).
package api

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/generator"
	"garydmenezes.com/mathgame/server/mathcore"
)

// MatrixCell is one (bitmap, difficulty-bucket) cell. Short JSON keys with
// omitempty: at ~200k cells the key names dominate the payload. A targetable
// cell carries the live example (Ex/Ans/Diff) and its pool count; an
// over-ceiling cell carries only Pool (if any) and Over; an unbuildable
// targetable cell carries neither example nor Over (Ex == "" is the marker).
type MatrixCell struct {
	Ex   string  `json:"e,omitempty"`
	Ans  string  `json:"a,omitempty"`
	Diff float64 `json:"d,omitempty"`
	Pool int     `json:"p,omitempty"`
	Over bool    `json:"o,omitempty"`
}

// MatrixRow is one bitmap's row of cells. Cells[i] is the bucket AxisLo+i; the
// row runs from AxisLo up to its own ceiling (extended to cover any over-ceiling
// pool rows), so most rows are far shorter than the full axis.
type MatrixRow struct {
	Bitmap uint64       `json:"bitmap"`
	Bits   []string     `json:"bits"`
	Ceil   int          `json:"ceil"`
	Cells  []MatrixCell `json:"cells"`
}

// BitmapMatrixData is the whole report. AxisLo is MinTargetDifficulty (3);
// AxisHi is the global max floor(MaxDiffForBitmap), extended to cover the
// highest counted over-ceiling pool bucket. PoolMax is the largest per-cell pool
// count, for the frontend heatmap's log/quantile scaling.
type BitmapMatrixData struct {
	AxisLo  int         `json:"axis_lo"`
	AxisHi  int         `json:"axis_hi"`
	PoolMax int         `json:"pool_max"`
	Rows    []MatrixRow `json:"rows"`
}

// Background-recompute state, mirroring admin_calibration.go: a dedup/liveness
// guard plus an atomic progress counter (bitmaps done / total) surfaced in the
// GET response headers so the page can show a percentage while polling.
var (
	bitmapMatrixComputing atomic.Bool
	bitmapMatrixDone      atomic.Int64
	bitmapMatrixTotal     atomic.Int64
)

// buildMatrixCell asks heuristic_2.0 for one problem in the (bitmap, target)
// cell and returns its DISPLAY expression, answer, and computed difficulty.
// BuildProblem returns the canonical grammar form (unspaced a/b), which would
// render as literal text; DisplayExpression skins it to \frac/\div/\times for
// KaTeX. The DRY seam shared by the full recompute and the per-cell reroll.
func buildMatrixCell(bitmap mathcore.ProblemType, target float64, rng *rand.Rand) (expr, answer string, diff float64, ok bool) {
	grammar, ans, err := generator.BuildProblem(bitmap, target, rng)
	if err != nil {
		return "", "", 0, false
	}
	return mathcore.DisplayExpression(grammar), ans, mathcore.ComputeProblemDifficulty(grammar, ""), true
}

// matrixCellKey is a (normalized bitmap, difficulty bucket) pool-count key.
type matrixCellKey struct {
	bitmap uint64
	bucket int
}

// computeBitmapMatrixReport builds the whole report over the given bitmap
// universe (injectable so tests can pass a tiny slice; production passes
// EnumerateValidBitmaps). It reads pool usage in one grouped query, then
// live-generates every targetable cell over a worker pool.
func (a *Api) computeBitmapMatrixReport(bitmaps []mathcore.ProblemType) (BitmapMatrixData, error) {
	valid := make(map[uint64]bool, len(bitmaps))
	for _, b := range bitmaps {
		valid[uint64(b)] = true
	}

	// Pool counts in one query, folded with the stamped→settings normalization:
	// stamping brackets magnitude (>=100 stamps LARGE_NUMBERS alone), but the
	// enumerated envelopes require LARGE ⇒ MEDIUM, so OR MEDIUM into any stamped
	// bitmap carrying LARGE before matching. Counts are exact-match (normalized
	// stamped bitmap == row envelope), not servability. WORD-stamped and other
	// non-enumerated rows fold to no row and are dropped.
	pool := map[matrixCellKey]int{}
	rowHiPool := map[uint64]int{} // highest counted bucket per bitmap
	rows, err := a.DB.Query(
		"SELECT problem_type_bitmap, CAST(ROUND(difficulty) AS SIGNED) AS bucket, COUNT(*) " +
			"FROM problems WHERE status = 'active' GROUP BY problem_type_bitmap, bucket")
	if err != nil {
		return BitmapMatrixData{}, err
	}
	for rows.Next() {
		var bm uint64
		var bucket, count int
		if rows.Scan(&bm, &bucket, &count) != nil {
			continue
		}
		norm := bm
		if mathcore.ProblemType(bm)&mathcore.LARGE_NUMBERS != 0 {
			norm |= uint64(mathcore.MEDIUM_NUMBERS)
		}
		if !valid[norm] {
			continue
		}
		pool[matrixCellKey{norm, bucket}] += count
		if bucket > rowHiPool[norm] {
			rowHiPool[norm] = bucket
		}
	}
	rows.Close()

	poolMax := 0
	for _, count := range pool {
		if count > poolMax {
			poolMax = count
		}
	}

	axisLo := int(mathcore.MinTargetDifficulty)
	axisHi := axisLo
	for _, b := range bitmaps {
		if c := int(math.Floor(mathcore.MaxDiffForBitmap(uint64(b)))); c > axisHi {
			axisHi = c
		}
	}
	for _, hi := range rowHiPool {
		if hi > axisHi {
			axisHi = hi
		}
	}

	// Worker pool over bitmaps: one *rand.Rand per worker (never shared — that
	// is data-race safety, not determinism; RNG is time-seeded, reproducibility
	// is not a goal).
	results := make([]MatrixRow, len(bitmaps))
	jobs := make(chan int)
	var wg sync.WaitGroup
	base := time.Now().UnixNano()
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			for idx := range jobs {
				results[idx] = buildMatrixRow(bitmaps[idx], axisLo, pool, rowHiPool, rng)
				bitmapMatrixDone.Add(1)
			}
		}(base + int64(w))
	}
	for i := range bitmaps {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return BitmapMatrixData{AxisLo: axisLo, AxisHi: axisHi, PoolMax: poolMax, Rows: results}, nil
}

// buildMatrixRow assembles one bitmap's row: a targetable cell (bucket <=
// ceiling) gets a live example; an over-ceiling cell is greyed with its pool
// count only. The row runs to max(ceiling, highest counted bucket) so
// over-ceiling pool rows stay visible.
func buildMatrixRow(bitmap mathcore.ProblemType, axisLo int, pool map[matrixCellKey]int, rowHiPool map[uint64]int, rng *rand.Rand) MatrixRow {
	ceil := int(math.Floor(mathcore.MaxDiffForBitmap(uint64(bitmap))))
	hi := ceil
	if h := rowHiPool[uint64(bitmap)]; h > hi {
		hi = h
	}
	cells := make([]MatrixCell, 0, hi-axisLo+1)
	for bucket := axisLo; bucket <= hi; bucket++ {
		cell := MatrixCell{Pool: pool[matrixCellKey{uint64(bitmap), bucket}]}
		if bucket > ceil {
			cell.Over = true // above the serving ceiling: not targetable
		} else if expr, ans, diff, ok := buildMatrixCell(bitmap, mathcore.TargetForBucket(uint64(bitmap), bucket), rng); ok {
			cell.Ex, cell.Ans, cell.Diff = expr, ans, diff
		}
		cells = append(cells, cell)
	}
	bits := mathcore.ProblemTypeToFeatures(bitmap)
	sort.Strings(bits)
	return MatrixRow{Bitmap: uint64(bitmap), Bits: bits, Ceil: ceil, Cells: cells}
}

// gzipJSON marshals v and gzip-compresses it, returning the compressed bytes
// stored (and served) verbatim.
func gzipJSON(v interface{}) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// adminBitmapMatrix serves the cached report. The body is the stored gzip bytes
// as-is (Content-Encoding: gzip); the envelope metadata — whether a report
// exists, whether a rebuild is in flight, and its progress — rides in response
// headers so the body stays a pure gzipped report the browser gunzips
// transparently.
func (a *Api) adminBitmapMatrix(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	c.Header("X-Computing", strconv.FormatBool(bitmapMatrixComputing.Load()))
	c.Header("X-Compute-Done", strconv.FormatInt(bitmapMatrixDone.Load(), 10))
	c.Header("X-Compute-Total", strconv.FormatInt(bitmapMatrixTotal.Load(), 10))

	var blob []byte
	var computedAt string
	err := a.DB.QueryRow("SELECT report, computed_at FROM bitmap_matrix_report WHERE id = 1").Scan(&blob, &computedAt)
	if err == sql.ErrNoRows {
		c.Header("X-Has-Report", "false")
		c.Status(http.StatusOK)
		return
	}
	if err != nil {
		glog.Errorf("%s bitmap-matrix cache read: %v", logPrefix, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, common.GetError("bitmap-matrix read failed"))
		return
	}
	c.Header("X-Has-Report", "true")
	c.Header("X-Computed-At", computedAt)
	c.Header("Content-Encoding", "gzip")
	c.Data(http.StatusOK, "application/json", blob)
}

// adminRecomputeBitmapMatrix rebuilds the report in the background and stores it
// gzipped, returning immediately. A second request while one is running is a
// no-op.
func (a *Api) adminRecomputeBitmapMatrix(c *gin.Context) {
	startBackgroundReport(a.DB, &bitmapMatrixComputing, common.GetLogPrefix(c), "bitmap-matrix",
		"INSERT INTO bitmap_matrix_report (id, report, computed_at) VALUES (1, ?, NOW()) "+
			"ON DUPLICATE KEY UPDATE report = VALUES(report), computed_at = VALUES(computed_at)",
		func() ([]byte, error) {
			bitmaps := a.bitmapMatrixBitmaps
			if bitmaps == nil {
				bitmaps = mathcore.EnumerateValidBitmaps()
			}
			// Progress counters are set here (inside the guard, so a lost
			// CompareAndSwap race never resets a live run's counters); a GET in
			// the brief window before this runs reads total 0, which the page
			// treats as 0%.
			bitmapMatrixTotal.Store(int64(len(bitmaps)))
			bitmapMatrixDone.Store(0)
			report, err := a.computeBitmapMatrixReport(bitmaps)
			if err != nil {
				return nil, err
			}
			return gzipJSON(report)
		})
	c.JSON(http.StatusOK, gin.H{"computing": true})
}

// adminBitmapMatrixCell live-regenerates a single cell (the per-cell ↻). It is
// ephemeral: the cache is never touched. 400 if the bitmap is invalid or the
// bucket is outside the targetable range (below the floor or above the ceiling).
func (a *Api) adminBitmapMatrixCell(c *gin.Context) {
	bm, err1 := strconv.ParseUint(c.Query("bitmap"), 10, 64)
	bucket, err2 := strconv.Atoi(c.Query("bucket"))
	if err1 != nil || err2 != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, common.GetError("bitmap and bucket must be integers"))
		return
	}
	pt := mathcore.ProblemType(bm)
	if !mathcore.ValidBitmap(pt) {
		c.AbortWithStatusJSON(http.StatusBadRequest, common.GetError("not a valid bitmap"))
		return
	}
	ceil := int(math.Floor(mathcore.MaxDiffForBitmap(bm)))
	if bucket < int(mathcore.MinTargetDifficulty) || bucket > ceil {
		c.AbortWithStatusJSON(http.StatusBadRequest, common.GetError("bucket is not targetable for this bitmap"))
		return
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	expr, ans, diff, ok := buildMatrixCell(pt, mathcore.TargetForBucket(bm, bucket), rng)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, common.GetError("could not build a problem for this cell"))
		return
	}
	c.JSON(http.StatusOK, MatrixCell{Ex: expr, Ans: ans, Diff: diff})
}

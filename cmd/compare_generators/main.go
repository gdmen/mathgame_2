// compare_generators is the informational Validation-B report for heuristic_2.0:
// the human-review companion to the CI B-gate in
// server/generator/heuristic2_test.go. It reads the real problem pool from a
// prod snapshot, finds the symbolic cells users actually get served and who
// fills them today, then puts fresh heuristic_2.0 output side by side with the
// stored heuristic_1.0 / llm_* rows in the same (bitmap, difficulty) cell so the
// pedagogy and naturalness can be eyeballed. It also reports how much of the
// symbolic LLM volume heuristic_2.0 could absorb (the LLM-offload story) and
// which cells it cannot reach in-window (the documented coarse-concept /
// floor-ceiling gaps).
//
// Read-only. Mirrors cmd/diagnose_generation's shape; needs a config with
// MySQL creds pointing at the snapshot.
//
// Usage:
//
//	./compare_generators -config=conf.json -cells=15 -samples=3
//	./compare_generators -config=conf.json -mode=matrix
//
// -mode=samples (default) is the volume-ranked head-to-head (top -cells).
// -mode=matrix swaps it for a complete per-bitmap difficulty coverage grid that
// represents every distinct bitmap in the pool, not just the busiest cells.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"sort"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	heuristic "garydmenezes.com/mathgame/server/generator"
	"garydmenezes.com/mathgame/server/mathcore"
)

const window = 1.5 // selection epsilon

type cell struct {
	bitmap uint64
	bucket int // ROUND(difficulty)
}

type cellInfo struct {
	cell
	byGen map[string]int // generator -> live row count
	total int
}

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON (MySQL creds for the snapshot)")
	topCells := flag.Int("cells", 15, "how many highest-volume symbolic cells to detail (samples mode)")
	samples := flag.Int("samples", 3, "example expressions per generator per cell (samples mode)")
	seed := flag.Int64("seed", 20260625, "rng seed for fresh heuristic_2.0 generation")
	mode := flag.String("mode", "samples", `report body: "samples" (volume-ranked head-to-head, top -cells) or "matrix" (complete per-bitmap difficulty coverage grid over every distinct bitmap)`)
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatal(err)
	}
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	rng := rand.New(rand.NewSource(*seed))

	cells := readGrid(db)

	// Symbolic cells only: heuristic_2.0 emits no WORD problems, so a WORD cell
	// is not a fair comparison (the LLM is the only generator there).
	var symbolic []cellInfo
	for _, ci := range cells {
		if ci.bitmap&uint64(mathcore.WORD) != 0 {
			continue
		}
		if ci.bucket < int(mathcore.MinTargetDifficulty) {
			continue
		}
		symbolic = append(symbolic, ci)
	}
	// Tiebreak past total so the order — and thus the shared-rng draw sequence
	// every report section consumes — is fully determined by -seed. readGrid
	// folds through a map (random iteration order), so without this the output
	// varies run-to-run despite the fixed seed.
	sort.Slice(symbolic, func(i, j int) bool {
		if symbolic[i].total != symbolic[j].total {
			return symbolic[i].total > symbolic[j].total
		}
		if symbolic[i].bitmap != symbolic[j].bitmap {
			return symbolic[i].bitmap < symbolic[j].bitmap
		}
		return symbolic[i].bucket < symbolic[j].bucket
	})

	reportOffload(symbolic, rng)
	switch *mode {
	case "matrix":
		reportMatrix(symbolic, rng)
	case "samples":
		reportHeadToHead(db, symbolic, *topCells, *samples, rng)
	default:
		glog.Fatalf("unknown -mode %q (want \"samples\" or \"matrix\")", *mode)
	}
	reportGaps(symbolic, rng)
}

// readGrid runs the issue's grouping query and folds it into per-cell counts.
func readGrid(db *sql.DB) []cellInfo {
	rows, err := db.Query(`SELECT problem_type_bitmap, ROUND(difficulty) AS db, generator, COUNT(*)
		FROM problems WHERE disabled = 0 GROUP BY problem_type_bitmap, db, generator`)
	if err != nil {
		glog.Fatalf("grid query: %v", err)
	}
	defer rows.Close()
	idx := map[cell]*cellInfo{}
	for rows.Next() {
		var bm uint64
		var bucket, count int
		var gen string
		if err := rows.Scan(&bm, &bucket, &gen, &count); err != nil {
			continue
		}
		k := cell{bm, bucket}
		ci := idx[k]
		if ci == nil {
			ci = &cellInfo{cell: k, byGen: map[string]int{}}
			idx[k] = ci
		}
		ci.byGen[gen] += count
		ci.total += count
	}
	out := make([]cellInfo, 0, len(idx))
	for _, ci := range idx {
		out = append(out, *ci)
	}
	return out
}

func isLLM(gen string) bool { return len(gen) >= 3 && gen[:3] == "llm" }

// targetFor is the difficulty heuristic_2.0 would actually be asked for in this
// cell: the bucket, clamped to the envelope's serving ceiling. Legacy rows can
// sit ABOVE MaxDiffForBitmap (old generators used operands beyond
// LargeMaxOperand), but target_difficulty is clamped to the ceiling, so the
// realistic ask — and the fair comparison — is min(bucket, ceiling).
func targetFor(bm uint64, bucket int) float64 {
	ceil := mathcore.MaxDiffForBitmap(bm)
	if float64(bucket) > ceil {
		return ceil
	}
	return float64(bucket)
}

// build asks heuristic_2.0 for a problem at the given target difficulty and
// returns the rendered expr and its computed difficulty.
func build(bm uint64, target float64, rng *rand.Rand) (string, float64, bool) {
	expr, _, err := heuristic.BuildProblem(mathcore.ProblemType(bm), target, rng)
	if err != nil {
		return "", 0, false
	}
	return expr, mathcore.ComputeProblemDifficulty(expr, ""), true
}

// reportOffload estimates how much currently-LLM-served symbolic volume
// heuristic_2.0 could absorb (one in-window probe per cell).
func reportOffload(symbolic []cellInfo, rng *rand.Rand) {
	var llmRows, absorbable, llmOnlyCells, llmOnlyCovered int
	for _, ci := range symbolic {
		llmVol := 0
		for gen, n := range ci.byGen {
			if isLLM(gen) {
				llmVol += n
			}
		}
		if llmVol == 0 {
			continue
		}
		llmRows += llmVol
		heuristicHere := ci.byGen["heuristic_1.0"] + ci.byGen["heuristic_2.0"]
		if heuristicHere == 0 {
			llmOnlyCells++ // a cell the old heuristic never filled
		}
		// Probe: can heuristic_2.0 hit this cell in-window? (best of a few tries)
		tgt := targetFor(ci.bitmap, ci.bucket)
		hit := false
		for i := 0; i < 8; i++ {
			if _, d, ok := build(ci.bitmap, tgt, rng); ok && math.Abs(d-tgt) <= window {
				hit = true
				break
			}
		}
		if hit {
			absorbable += llmVol
			if heuristicHere == 0 {
				llmOnlyCovered++
			}
		}
	}
	fmt.Println("=== LLM-offload (symbolic cells) ===")
	fmt.Printf("symbolic LLM rows in pool:           %d\n", llmRows)
	if llmRows > 0 {
		fmt.Printf("absorbable by heuristic_2.0 in-window: %d (%.1f%%)\n", absorbable, 100*float64(absorbable)/float64(llmRows))
	}
	fmt.Printf("llm-only symbolic cells (old heuristic couldn't fill): %d; now coverable by 2.0: %d\n\n", llmOnlyCells, llmOnlyCovered)
}

// reportHeadToHead prints stored 1.0/llm expressions next to fresh 2.0 output
// in the highest-volume symbolic cells, for eyeballing pedagogy/naturalness.
func reportHeadToHead(db *sql.DB, symbolic []cellInfo, topCells, samples int, rng *rand.Rand) {
	fmt.Println("=== Side-by-side samples (highest-volume symbolic cells) ===")
	shown := 0
	for _, ci := range symbolic {
		if shown >= topCells {
			break
		}
		shown++
		feats := mathcore.ProblemTypeToFeatures(mathcore.ProblemType(ci.bitmap))
		fmt.Printf("\nCELL bitmap=%d diff~%d  total=%d  %v\n", ci.bitmap, ci.bucket, ci.total, feats)
		for _, gen := range sortedGens(ci.byGen) {
			for _, ex := range examples(db, ci.bitmap, ci.bucket, gen, samples) {
				fmt.Printf("  %-14s (db)  %-32s d=%.1f\n", gen, ex.expr, ex.diff)
			}
		}
		tgt := targetFor(ci.bitmap, ci.bucket)
		for i := 0; i < samples; i++ {
			if expr, d, ok := build(ci.bitmap, tgt, rng); ok {
				flag := ""
				if math.Abs(d-tgt) > window {
					flag = "  [out-of-window]"
				}
				fmt.Printf("  %-14s (fresh) %-32s d=%.1f (target %.1f)%s\n", "heuristic_2.0", expr, d, tgt, flag)
			}
		}
	}
	fmt.Println()
}

// reportMatrix prints one row per distinct symbolic bitmap (not the volume-
// ranked top-N), and for each integer difficulty bucket in that bitmap's
// serving range probes whether heuristic_2.0 lands in-window. Unlike the
// head-to-head, every distinct feature combination in the pool is represented,
// however low its volume — the point is coverage at a glance across the whole
// bit space, not expression naturalness.
func reportMatrix(symbolic []cellInfo, rng *rand.Rand) {
	const tries = 12

	// Fold per-(bitmap,bucket) cells into per-bitmap rows, carrying total
	// volume so rare bitmaps are still visible (and sortable) alongside busy
	// ones. The serving range comes from the envelope, not the pool, so a
	// bucket the pool never filled is still probed.
	type row struct {
		bitmap uint64
		vol    int
	}
	idx := map[uint64]*row{}
	for _, ci := range symbolic {
		r := idx[ci.bitmap]
		if r == nil {
			r = &row{bitmap: ci.bitmap}
			idx[ci.bitmap] = r
		}
		r.vol += ci.total
	}
	rows := make([]*row, 0, len(idx))
	for _, r := range idx {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].vol != rows[j].vol {
			return rows[i].vol > rows[j].vol
		}
		return rows[i].bitmap < rows[j].bitmap
	})

	lo := int(mathcore.MinTargetDifficulty)
	hi := lo
	for _, r := range rows {
		if c := int(math.Floor(mathcore.MaxDiffForBitmap(r.bitmap))); c > hi {
			hi = c
		}
	}

	fmt.Println("=== Coverage matrix (heuristic_2.0 in-window hit per bitmap x difficulty) ===")
	fmt.Println("legend: # in-window   . reachable but out-of-window   x unbuildable   (blank) above serving ceiling")
	fmt.Printf("%-7s ", "diff")
	for d := lo; d <= hi; d++ {
		fmt.Printf("%-2d", d%100)
	}
	fmt.Printf("  %-4s %-7s %s\n", "hit", "vol", "features")

	var totHit, totProbed int
	for _, r := range rows {
		ceil := int(math.Floor(mathcore.MaxDiffForBitmap(r.bitmap)))
		var sb []byte
		hits, probed := 0, 0
		for d := lo; d <= hi; d++ {
			if d > ceil {
				sb = append(sb, ' ', ' ')
				continue
			}
			tgt := targetFor(r.bitmap, d)
			bestErr, built := math.MaxFloat64, false
			for i := 0; i < tries; i++ {
				if _, dd, ok := build(r.bitmap, tgt, rng); ok {
					built = true
					if e := math.Abs(dd - tgt); e < bestErr {
						bestErr = e
					}
				}
			}
			probed++
			switch {
			case !built:
				sb = append(sb, 'x', ' ')
			case bestErr <= window:
				hits++
				sb = append(sb, '#', ' ')
			default:
				sb = append(sb, '.', ' ')
			}
		}
		totHit += hits
		totProbed += probed
		feats := mathcore.ProblemTypeToFeatures(mathcore.ProblemType(r.bitmap))
		fmt.Printf("bm=%-4d %s %3d/%-3d %-7d %v\n", r.bitmap, string(sb), hits, probed, r.vol, feats)
	}
	if totProbed > 0 {
		fmt.Printf("\n%d bitmaps, %d in-serving-range buckets probed, %d in-window (%.1f%%)\n\n",
			len(rows), totProbed, totHit, 100*float64(totHit)/float64(totProbed))
	}
}

// reportGaps lists symbolic cells heuristic_2.0 cannot reach in-window, with an
// example, so the coarse-concept / floor-ceiling gaps are concrete.
func reportGaps(symbolic []cellInfo, rng *rand.Rand) {
	fmt.Println("=== Coverage gaps (symbolic cells heuristic_2.0 misses in-window) ===")
	type gap struct {
		ci   cellInfo
		expr string
		d    float64
	}
	var gaps []gap
	var overCeiling int // legacy buckets above the serving ceiling (not targetable)
	for _, ci := range symbolic {
		if float64(ci.bucket) > mathcore.MaxDiffForBitmap(ci.bitmap)+window {
			overCeiling++
			continue // not a builder gap: target_difficulty is clamped below here
		}
		tgt := targetFor(ci.bitmap, ci.bucket)
		bestErr, bestExpr, bestD := math.MaxFloat64, "", 0.0
		for i := 0; i < 12; i++ {
			expr, d, ok := build(ci.bitmap, tgt, rng)
			if !ok {
				continue
			}
			if e := math.Abs(d - tgt); e < bestErr {
				bestErr, bestExpr, bestD = e, expr, d
			}
		}
		if bestErr > window {
			gaps = append(gaps, gap{ci, bestExpr, bestD})
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].ci.total != gaps[j].ci.total {
			return gaps[i].ci.total > gaps[j].ci.total
		}
		if gaps[i].ci.bitmap != gaps[j].ci.bitmap {
			return gaps[i].ci.bitmap < gaps[j].ci.bitmap
		}
		return gaps[i].ci.bucket < gaps[j].ci.bucket
	})
	for i, g := range gaps {
		if i >= 25 {
			fmt.Printf("... and %d more gap cells\n", len(gaps)-25)
			break
		}
		tgt := targetFor(g.ci.bitmap, g.ci.bucket)
		kind := "floor (envelope min > target)"
		if g.d < tgt {
			kind = "ceiling/coarse (can't reach up to target)"
		}
		fmt.Printf("  bitmap=%d diff~%d(target %.1f) vol=%d  best=%q d=%.1f  [%s]  %v\n",
			g.ci.bitmap, g.ci.bucket, tgt, g.ci.total, g.expr, g.d, kind,
			mathcore.ProblemTypeToFeatures(mathcore.ProblemType(g.ci.bitmap)))
	}
	if len(gaps) == 0 {
		fmt.Println("  (none within the serving ceiling)")
	}
	fmt.Printf("(%d legacy over-ceiling buckets skipped — above MaxDiffForBitmap, not targetable)\n\n", overCeiling)
}

type ex struct {
	expr string
	diff float64
}

func examples(db *sql.DB, bitmap uint64, bucket int, gen string, limit int) []ex {
	rows, err := db.Query(
		`SELECT expression, difficulty FROM problems
		 WHERE disabled=0 AND problem_type_bitmap=? AND ROUND(difficulty)=? AND generator=?
		 ORDER BY id LIMIT ?`,
		bitmap, bucket, gen, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []ex
	for rows.Next() {
		var e ex
		if err := rows.Scan(&e.expr, &e.diff); err == nil {
			out = append(out, e)
		}
	}
	return out
}

func sortedGens(byGen map[string]int) []string {
	gens := make([]string, 0, len(byGen))
	for g := range byGen {
		gens = append(gens, g)
	}
	sort.Strings(gens)
	return gens
}

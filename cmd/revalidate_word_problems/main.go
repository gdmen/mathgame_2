// revalidate_word_problems re-stamps the topic bits of WORD rows from the
// LLM validator's observed-features line, replacing the legacy self-reported
// topic bits the bitmap backfill preserved. The parser cannot see concepts
// inside prose, so the validator is the only authority for what a word
// problem actually exercises (including chained_operations: multi-step
// prose carries no symbolic operators to count).
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
//
// Semantics:
//   - One validator call per enabled WORD row, hardcoded to the cheap
//     model at its default reasoning effort: the live validator's model
//     would cost hundreds of dollars, and reduced effort measurably fails
//     (200-row ground truth: minimal hallucinated features on ~70 rows;
//     low silently missed 9 of 25 multi-step rows). Throughput comes from
//     -workers, not shallower calls.
//   - new bitmap = parser-detected shape bits | validator feature bits,
//     plus the stored WORD bit (a prose row must never widen its audience
//     by losing WORD). Legacy self-reported topic bits do not survive.
//   - Validator answer mismatches, constraint NOs, and API errors leave the
//     row unchanged and are listed in the report for review/retry.
//   - Bitmap-only writes: expression/answer/explanation are never touched.
//   - Re-runs are cheap on the DB but re-spend LLM calls; resume an
//     interrupted run with -start-id=<printed resume_from>.
//
// Usage:
//
//	./revalidate_word_problems -config=conf.json -dry-run -limit=100
//	./revalidate_word_problems -config=conf.json -workers=4
//	./revalidate_word_problems -config=conf.json -start-id=123456
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/llm_generator"
)

// newBitmapFor combines the parser's shape bits (magnitude, word, any
// symbolic structure) with the validator's observed topic features.
func newBitmapFor(expr string, features []string) uint64 {
	return api.DetectProblemTypeBitmap(expr) | uint64(api.FeaturesToProblemType(features))
}

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	dryRun := flag.Bool("dry-run", false, "don't write; print what would change")
	limit := flag.Int("limit", 0, "process only this many rows (0 = all)")
	workers := flag.Int("workers", 16, "concurrent validator calls (retry wrapper absorbs rate-limit pushback)")
	startID := flag.Int64("start-id", 0, "skip rows below this id (resume)")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatal(err)
	}
	// ValidateWordProblem reads ./conf.json on every call and requires a
	// complete config. Fail fast here instead of 190K times in the workers.
	if probe, err := common.ReadConfig("conf.json"); err != nil {
		glog.Fatalf("ValidateWordProblem reads ./conf.json - run from the repo root: %v", err)
	} else if err := probe.Validate(); err != nil {
		glog.Fatalf("./conf.json incomplete for the validator: %v", err)
	}

	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	query := fmt.Sprintf(
		`SELECT id, expression, answer, explanation, problem_type_bitmap FROM problems
		 WHERE disabled = 0 AND (problem_type_bitmap & %d) <> 0 AND id >= ? ORDER BY id`,
		uint64(api.WORD))
	if *limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, *limit)
	}
	rows, err := db.Query(query, *startID)
	if err != nil {
		glog.Fatalf("query problems: %v", err)
	}

	type rec struct {
		id          uint32
		expr        string
		answer      string
		explanation string
		oldBitmap   uint64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.expr, &r.answer, &r.explanation, &r.oldBitmap); err != nil {
			glog.Errorf("scan: %v", err)
			continue
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		glog.Fatalf("rows iteration: %v", err)
	}
	rows.Close()

	// The validator judges compliance against generation constraints; with
	// everything enabled only the hard caps remain (operand size, chain
	// length, unknown rules, closed-world notation) - legacy rows exceeding
	// those land in the constraint-NO bucket below.
	constraints := api.BuildBitConstraints(api.ALL_PROBLEM_TYPES)

	var (
		mu                                        sync.Mutex
		done, updated, unchanged, gainedChained   int
		mismatchRows, constraintNoRows, errorRows []uint32
		inflight                                  = map[uint32]bool{}
		progressStep                              = len(recs) / 100
	)
	if progressStep < 100 {
		progressStep = 100
	}
	// finish updates shared state and prints progress. resume_from is the
	// lowest id not yet completed: with out-of-order workers, the printed
	// last-finished id may be above rows still in flight, and resuming there
	// would silently skip them.
	finish := func(id uint32, apply func()) {
		mu.Lock()
		defer mu.Unlock()
		delete(inflight, id)
		if apply != nil {
			apply()
		}
		done++
		if done%progressStep == 0 || done == len(recs) {
			resumeFrom := uint64(id) + 1
			for f := range inflight {
				if uint64(f) < resumeFrom {
					resumeFrom = uint64(f)
				}
			}
			fmt.Fprintf(os.Stderr, "  %d/%d (%.1f%%) resume_from=%d\n",
				done, len(recs), 100*float64(done)/float64(len(recs)), resumeFrom)
		}
	}

	jobs := make(chan rec)
	var wg sync.WaitGroup
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				p := &llm_generator.Problem{
					Expression:  r.expr,
					Answer:      r.answer,
					Explanation: r.explanation,
				}
				features, err := llm_generator.ValidateWordProblemWithModel(p, constraints, api.ValidatorFeatureNames, openai.GPT5Nano)
				if err != nil {
					switch {
					case errors.Is(err, llm_generator.ErrEnvelopeMismatch):
						finish(r.id, func() { constraintNoRows = append(constraintNoRows, r.id) })
					case strings.Contains(err.Error(), "MISMATCH"):
						finish(r.id, func() { mismatchRows = append(mismatchRows, r.id) })
					default:
						finish(r.id, func() { errorRows = append(errorRows, r.id) })
					}
					continue
				}

				// Never widen the audience: the stored WORD bit survives even
				// if a lexer-failing prose row's detector or validator misses it.
				newBitmap := newBitmapFor(r.expr, features) | (r.oldBitmap & uint64(api.WORD))
				if newBitmap == r.oldBitmap {
					finish(r.id, func() { unchanged++ })
					continue
				}
				if *dryRun {
					fmt.Printf("DRY id=%d bitmap %d (%v) -> %d (%v)\n",
						r.id, r.oldBitmap, api.ProblemTypeToFeatures(api.ProblemType(r.oldBitmap)),
						newBitmap, api.ProblemTypeToFeatures(api.ProblemType(newBitmap)))
				} else if _, err := db.Exec(
					`UPDATE problems SET problem_type_bitmap = ? WHERE id = ?`,
					newBitmap, r.id,
				); err != nil {
					glog.Errorf("update id=%d: %v", r.id, err)
					finish(r.id, func() { errorRows = append(errorRows, r.id) })
					continue
				}
				gained := newBitmap&uint64(api.CHAINED_OPERATIONS) != 0 &&
					r.oldBitmap&uint64(api.CHAINED_OPERATIONS) == 0
				finish(r.id, func() {
					updated++
					if gained {
						gainedChained++
					}
				})
			}
		}()
	}
	for _, r := range recs {
		mu.Lock()
		inflight[r.id] = true
		mu.Unlock()
		jobs <- r
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("\n=== revalidate_word_problems report ===\n")
	fmt.Printf("total=%d updated=%d unchanged=%d dry_run=%v\n",
		len(recs), updated, unchanged, *dryRun)
	fmt.Printf("rows gaining CHAINED_OPERATIONS: %d\n", gainedChained)
	fmt.Printf("validator answer mismatches (REVIEW; rows unchanged): %d rows", len(mismatchRows))
	printIDList(mismatchRows)
	fmt.Printf("constraint NOs (legacy rows exceeding generation caps; rows unchanged): %d rows", len(constraintNoRows))
	printIDList(constraintNoRows)
	fmt.Printf("validator/update errors (retry with -start-id; rows unchanged): %d rows", len(errorRows))
	printIDList(errorRows)
	os.Exit(0)
}

func printIDList(ids []uint32) {
	const maxShown = 50
	if len(ids) == 0 {
		fmt.Println()
		return
	}
	fmt.Print(": ")
	for i, id := range ids {
		if i >= maxShown {
			fmt.Printf("... (+%d more)", len(ids)-maxShown)
			break
		}
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Print(id)
	}
	fmt.Println()
}

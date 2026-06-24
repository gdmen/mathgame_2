// backfill_symbolic_expression fills the symbolic_expression of legacy WORD
// rows that predate llm_0.5. For each word problem with an empty
// symbolic_expression it asks the LLM to derive the bare computation from the
// prose, verifies the form lexes and evaluates to the stored answer, then
// writes the form, a bitmap re-derived from the form (so chained/magnitude/
// concept bits the prose hides are stamped), the form-based difficulty, and the
// current DifficultyVersion. This supersedes revalidate_word_problems for any
// row it touches: the form is a deterministic, free, and more complete source
// of bits than the validator's feature list.
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
//
// Semantics:
//   - One LLM derive call per WORD row with no form, hardcoded to the cheap
//     model. Throughput comes from -workers, not shallower calls.
//   - The answer is NOT sent to the model (so it transcribes rather than works
//     backward); the form is then checked in-code against the stored answer.
//   - A form that fails to lex or doesn't evaluate to the answer leaves the row
//     unchanged and is listed for review/retry.
//   - new bitmap = WORD | bits detected from the form. The stored answer and
//     prose expression are never touched.
//   - Re-runs are cheap on the DB but re-spend LLM calls; resume an interrupted
//     run with -start-id=<printed resume_from>.
//
// Usage:
//
//	./backfill_symbolic_expression -config=conf.json -dry-run -limit=100
//	./backfill_symbolic_expression -config=conf.json -workers=8
//	./backfill_symbolic_expression -config=conf.json -start-id=123456
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/llm_generator"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	dryRun := flag.Bool("dry-run", false, "don't write; print what would change")
	limit := flag.Int("limit", 0, "process only this many rows (0 = all)")
	workers := flag.Int("workers", 16, "concurrent derive calls (retry wrapper absorbs rate-limit pushback)")
	startID := flag.Int64("start-id", 0, "skip rows below this id (resume)")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatal(err)
	}
	// DeriveSymbolicExpression reads ./conf.json on every call and requires a
	// complete config. Fail fast here instead of once per row in the workers.
	if probe, err := common.ReadConfig("conf.json"); err != nil {
		glog.Fatalf("DeriveSymbolicExpression reads ./conf.json - run from the repo root: %v", err)
	} else if err := probe.Validate(); err != nil {
		glog.Fatalf("./conf.json incomplete for the LLM call: %v", err)
	}

	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	// The real run streams in id order so -start-id can resume it. A dry-run
	// with -limit samples randomly instead, so the spot-check spans generators,
	// difficulties, and bitmaps rather than the oldest id cluster.
	base := fmt.Sprintf(
		`SELECT id, expression, answer, problem_type_bitmap, difficulty, generator FROM problems
		 WHERE disabled = 0 AND (problem_type_bitmap & %d) <> 0 AND symbolic_expression = ''`,
		uint64(api.WORD))
	var query string
	var args []any
	if *dryRun && *limit > 0 {
		query = fmt.Sprintf("%s ORDER BY RAND() LIMIT %d", base, *limit)
	} else {
		query = base + " AND id >= ? ORDER BY id"
		args = append(args, *startID)
		if *limit > 0 {
			query = fmt.Sprintf("%s LIMIT %d", query, *limit)
		}
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		glog.Fatalf("query problems: %v", err)
	}

	type rec struct {
		id        uint32
		expr      string
		answer    string
		oldBitmap uint64
		oldDiff   float64
		generator string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.expr, &r.answer, &r.oldBitmap, &r.oldDiff, &r.generator); err != nil {
			glog.Errorf("scan: %v", err)
			continue
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		glog.Fatalf("rows iteration: %v", err)
	}
	rows.Close()
	fmt.Fprintf(os.Stderr, "deriving symbolic_expression for %d word rows\n", len(recs))

	var (
		mu                         sync.Mutex
		done, updated, diffChanged int
		invalidRows, errorRows     []uint32
		bitmapChanged              []uint32
		inflight                   = map[uint32]bool{}
		progressStep               = len(recs) / 100
	)
	if progressStep < 100 {
		progressStep = 100
	}
	// resume_from is the lowest id not yet completed: with out-of-order workers
	// the printed last-finished id may be above rows still in flight, and
	// resuming there would silently skip them.
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
				p := &llm_generator.Problem{Expression: r.expr, Answer: r.answer}
				form, err := llm_generator.DeriveSymbolicExpressionWithModel(p, openai.GPT5Nano)
				if err != nil {
					finish(r.id, func() { errorRows = append(errorRows, r.id) })
					continue
				}
				// VerifyAnswer/AdmitExpression reduce a "? = 100 - 25"-style
				// label and verify the form lexes and evaluates to the answer.
				if err := api.VerifyAnswer(form, r.answer); err != nil {
					finish(r.id, func() { invalidRows = append(invalidRows, r.id) })
					continue
				}
				adm := api.AdmitExpression(form)
				newBitmap := api.NormalizeProblemBitmap(uint64(api.WORD) | api.WordFormBitmap(adm.Bitmap))
				newDiff := api.ComputeProblemDifficulty(r.expr, adm.Expr)
				bmChanged := newBitmap != r.oldBitmap
				dDelta := newDiff - r.oldDiff
				if dDelta < 0 {
					dDelta = -dDelta
				}
				apply := func() {
					updated++
					if bmChanged {
						bitmapChanged = append(bitmapChanged, r.id)
					}
					if dDelta > 0.01 {
						diffChanged++
					}
				}

				if *dryRun {
					fmt.Printf("DRY id=%d gen=%s form=%q diff %.2f->%.2f bitmap %d (%v) -> %d (%v)  %s\n",
						r.id, r.generator, adm.Expr, r.oldDiff, newDiff,
						r.oldBitmap, api.ProblemTypeToFeatures(api.ProblemType(r.oldBitmap)),
						newBitmap, api.ProblemTypeToFeatures(api.ProblemType(newBitmap)), r.expr)
					finish(r.id, apply)
					continue
				}
				if _, err := db.Exec(
					`UPDATE problems SET symbolic_expression = ?, problem_type_bitmap = ?, difficulty = ?, difficulty_version = ? WHERE id = ?`,
					adm.Expr, newBitmap, newDiff, api.DifficultyVersion, r.id,
				); err != nil {
					glog.Errorf("update id=%d: %v", r.id, err)
					finish(r.id, func() { errorRows = append(errorRows, r.id) })
					continue
				}
				finish(r.id, apply)
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

	fmt.Printf("\n=== backfill_symbolic_expression report ===\n")
	fmt.Printf("total=%d updated=%d dry_run=%v\n", len(recs), updated, *dryRun)
	fmt.Printf("  of which difficulty changed: %d rows\n", diffChanged)
	fmt.Printf("  of which bitmap changed: %d rows", len(bitmapChanged))
	printIDList(bitmapChanged)
	fmt.Printf("invalid forms (unlexable or != answer; rows unchanged): %d rows", len(invalidRows))
	printIDList(invalidRows)
	fmt.Printf("derive/update errors (retry with -start-id; rows unchanged): %d rows", len(errorRows))
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

// recompute_problem_type_bitmap walks all rows in the problems table and
// restamps problem_type_bitmap from the expression via the admission
// pipeline's detection stages (normalize -> lex -> rewrite -> detect). Run BEFORE
// recompute_problem_difficulty in the deploy sequence: the lone-letter
// rewrite mutates expression text, and difficulty must be computed from the
// final expressions.
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
//
// Semantics:
//   - SET, not OR: the detected bitmap REPLACES the stored one, so re-runs
//     are stable and legacy false-positive bits don't survive forever.
//   - WORD rows keep their existing legacy topic bits (the 7 original
//     self-reported bits) OR'd onto the detected shape bits: the parser
//     can't see topics inside prose, and re-validating 300K rows through the
//     LLM is not worth it for legacy data.
//   - Lone-letter rewrite: a single bare variable becomes '?'
//     (12 - x = 5 -> 12 - ? = 5) in the expression - the first time this
//     tool mutates expression text. The same standalone-letter substitution
//     is attempted in the explanation. Rewritten rows are listed for
//     spot-checking.
//
// Reports (always printed):
//   - lexer census: out-of-alphabet rows grouped by offending token
//   - zero-bitmap rows: under subset selection a zero bitmap matches every
//     user, so these are flagged for review/disable (selection SQL also
//     excludes them defensively)
//   - rewrite list + per-stage counts
//
// Usage:
//
//	./recompute_problem_type_bitmap -config=conf.json -dry-run   (census only)
//	./recompute_problem_type_bitmap -config=conf.json
//	./recompute_problem_type_bitmap -config=conf.json -limit=100
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

// legacyTopicMask is the original 7 self-reported bits preserved on WORD rows.
const legacyTopicMask = uint64(api.ADDITION | api.SUBTRACTION | api.MULTIPLICATION |
	api.DIVISION | api.FRACTIONS | api.NEGATIVES | api.WORD)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	dryRun := flag.Bool("dry-run", false, "don't write; print the census and what would change")
	limit := flag.Int("limit", 0, "process only this many rows (0 = all)")
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

	query := `SELECT id, expression, answer, explanation, problem_type_bitmap FROM problems ORDER BY id`
	if *limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, *limit)
	}
	rows, err := db.Query(query)
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

	var (
		total, updated, unchanged, rewritten        int
		lexFailed, zeroBitmap, unknownRuleViolation int
		tokenCensus                                 = map[string]int{}
		zeroRows, rewriteRows, lexRows, unknownRows []uint32
	)

	for _, r := range recs {
		total++
		newExpr := r.expr
		newAnswer := r.answer
		newExplanation := r.explanation

		// Same admission pipeline the live insert paths use - one source of
		// truth for stamping and the lone-letter splice (which preserves the
		// stored text's original notation).
		adm := api.AdmitExpression(r.expr)
		var newBitmap uint64
		switch adm.RejectStage {
		case "":
			newExpr = adm.Expr
			newBitmap = adm.Bitmap
			if adm.RewroteLetter != 0 {
				rewritten++
				rewriteRows = append(rewriteRows, r.id)
				newAnswer = api.RewriteLetterInProse(r.answer, adm.RewroteLetter)
				newExplanation = api.RewriteLetterInProse(r.explanation, adm.RewroteLetter)
			}
		case "lexer":
			lexFailed++
			tokenCensus[adm.RejectWhy]++
			lexRows = append(lexRows, r.id)
			// Out-of-alphabet rows still get restamped via the fallback
			// feature extraction inside DetectProblemTypeBitmap, but they
			// are surfaced here for review/disable.
			newBitmap = api.DetectProblemTypeBitmap(r.expr)
		default: // unknown_rules: multi-unknown legacy rows, flagged for review
			unknownRuleViolation++
			unknownRows = append(unknownRows, r.id)
			newBitmap = api.DetectProblemTypeBitmap(r.expr)
		}

		if newBitmap&uint64(api.WORD) != 0 {
			// Preserve legacy self-reported topic bits on WORD rows.
			newBitmap |= r.oldBitmap & legacyTopicMask
		}
		if newBitmap == 0 {
			zeroBitmap++
			zeroRows = append(zeroRows, r.id)
		}

		if newBitmap == r.oldBitmap && newExpr == r.expr && newAnswer == r.answer && newExplanation == r.explanation {
			unchanged++
			continue
		}
		updated++
		if *dryRun {
			if newExpr != r.expr {
				fmt.Printf("DRY id=%d expr %q -> %q bitmap %d -> %d\n", r.id, r.expr, newExpr, r.oldBitmap, newBitmap)
			}
			continue
		}
		if _, err := db.Exec(
			`UPDATE problems SET expression = ?, answer = ?, explanation = ?, problem_type_bitmap = ? WHERE id = ?`,
			newExpr, newAnswer, newExplanation, newBitmap, r.id,
		); err != nil {
			glog.Errorf("update id=%d: %v", r.id, err)
		}
	}

	fmt.Printf("\n=== recompute_problem_type_bitmap report ===\n")
	fmt.Printf("total=%d updated=%d unchanged=%d dry_run=%v\n", total, updated, unchanged, *dryRun)
	fmt.Printf("lone-letter rewrites: %d rows", rewritten)
	printIDList(rewriteRows)
	fmt.Printf("lexer-rejected (out of alphabet): %d rows", lexFailed)
	printIDList(lexRows)
	fmt.Printf("zero-bitmap (REVIEW: subset-matches nothing; selection excludes them): %d rows", zeroBitmap)
	printIDList(zeroRows)
	fmt.Printf("unknown-rule violations (REVIEW: multi-unknown legacy rows): %d rows", unknownRuleViolation)
	printIDList(unknownRows)

	fmt.Printf("\nlexer token census (offending token -> row count):\n")
	type kv struct {
		tok string
		n   int
	}
	var census []kv
	for tok, n := range tokenCensus {
		census = append(census, kv{tok, n})
	}
	sort.Slice(census, func(i, j int) bool { return census[i].n > census[j].n })
	for _, e := range census {
		fmt.Printf("  %6d  %q\n", e.n, e.tok)
	}
	if len(census) == 0 {
		fmt.Println("  (none - entire pool lexes clean)")
	}
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

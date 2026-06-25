// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here (bits, formula, pipeline, masks) REQUIRE updating that
// doc in the same PR. Formula changes also require a DifficultyVersion bump.
// recompute_problem_difficulty walks all rows in the problems table, computes
// the universal difficulty via
// mathcore.ComputeProblemDifficulty(expression, symbolic_expression), and writes it
// back to the difficulty column.
//
// Safe to run repeatedly. Idempotent. Use after deploying the universal
// difficulty change to migrate existing problems from their legacy pinned
// values (target_difficulty when generated) to computed values.
//
// Usage:
//
//	./recompute_problem_difficulty -config=conf.json
//	./recompute_problem_difficulty -config=conf.json -dry-run
//	./recompute_problem_difficulty -config=conf.json -limit=100    (test on a slice)
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	dryRun := flag.Bool("dry-run", false, "don't write; just print what would change")
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

	query := `SELECT id, expression, symbolic_expression, difficulty, difficulty_version FROM problems ORDER BY id`
	if *limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, *limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		glog.Fatalf("query problems: %v", err)
	}
	defer rows.Close()

	var updated, unchanged, skipped, total int
	var deltaSum float64
	// Collect rows first so we can close the query cursor before updating.
	type rec struct {
		id       uint32
		expr     string
		symbolic string
		oldDiff  float64
		oldVer   string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.expr, &r.symbolic, &r.oldDiff, &r.oldVer); err != nil {
			glog.Errorf("scan: %v", err)
			continue
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		glog.Fatalf("rows iteration: %v", err)
	}
	rows.Close()

	// Print progress every progressStep rows so a multi-minute run isn't silent.
	// Stderr keeps stdout clean for dry-run row diffs.
	fmt.Fprintf(os.Stderr, "loaded %d rows; processing...\n", len(recs))
	progressStep := len(recs) / 100
	if progressStep < 1000 {
		progressStep = 1000
	}
	for _, r := range recs {
		total++
		action, newDiff, err := recomputeProblemRow(db, r.id, r.expr, r.symbolic, r.oldDiff, r.oldVer, *dryRun)
		if err != nil {
			glog.Error(err)
			continue
		}
		switch action {
		case actionSkipped:
			skipped++
		case actionStamped:
			if *dryRun {
				fmt.Printf("id=%d expr=%q: value unchanged (%.2f), version %q -> %q\n", r.id, r.expr, r.oldDiff, r.oldVer, mathcore.DifficultyVersion)
			}
			unchanged++
		case actionUpdated:
			delta := newDiff - r.oldDiff
			deltaSum += delta
			if *dryRun {
				fmt.Printf("id=%d expr=%q: %.2f -> %.2f (%+.2f) ver=%q->%q\n", r.id, r.expr, r.oldDiff, newDiff, delta, r.oldVer, mathcore.DifficultyVersion)
			}
			updated++
		}
		if total%progressStep == 0 || total == len(recs) {
			fmt.Fprintf(os.Stderr, "  %d/%d (%.1f%%)\n", total, len(recs), 100*float64(total)/float64(len(recs)))
		}
	}

	fmt.Printf("\nRecompute summary:\n")
	fmt.Printf("  total:     %d\n", total)
	fmt.Printf("  updated:   %d\n", updated)
	fmt.Printf("  unchanged: %d\n", unchanged)
	fmt.Printf("  skipped:   %d (already at version %s)\n", skipped, mathcore.DifficultyVersion)
	if updated > 0 {
		fmt.Printf("  avg delta: %+.2f\n", deltaSum/float64(updated))
	}
	if *dryRun {
		fmt.Println("\n(dry run; no changes written)")
	}
	os.Exit(0)
}
